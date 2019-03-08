package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sort"
	"time"
)

func AssertValidTransaction(tx *Transaction) error {
	if tx.ID == common.ZeroTransactionID {
		return errors.New("tx must not be empty")
	}

	if tx.Sender == common.ZeroAccountID || tx.Creator == common.ZeroAccountID {
		return errors.New("tx must have sender or creator")
	}

	if len(tx.ParentIDs) == 0 {
		return errors.New("tx must have parents")
	}

	// Check that parents are lexicographically sorted, and are unique.
	set := make(map[common.TransactionID]struct{})

	for i := len(tx.ParentIDs) - 1; i > 0; i-- {
		if bytes.Compare(tx.ParentIDs[i-1][:], tx.ParentIDs[i][:]) >= 0 {
			return errors.New("tx must have sorted parent ids")
		}

		if _, duplicate := set[tx.ParentIDs[i]]; duplicate {
			return errors.New("tx must not have duplicate parent ids")
		}

		set[tx.ParentIDs[i]] = struct{}{}
	}

	sort.SliceIsSorted(tx.ParentIDs, func(i, j int) bool {
		return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
	})

	if tx.Tag != sys.TagNop && len(tx.Payload) == 0 {
		return errors.New("tx must have payload if not a nop transaction")
	}

	if tx.Tag == sys.TagNop && len(tx.Payload) != 0 {
		return errors.New("tx must have no payload if is a nop transaction")
	}

	err := eddsa.Verify(tx.Creator[:], append([]byte{tx.Tag}, tx.Payload...), tx.CreatorSignature[:])
	if err != nil {
		return errors.New("tx has invalid creator signature")
	}

	cpy := *tx
	cpy.SenderSignature = common.ZeroSignature

	err = eddsa.Verify(tx.Sender[:], cpy.Write(), tx.SenderSignature[:])
	if err != nil {
		return errors.New("tx has either invalid sender signature")
	}

	if len(tx.DifficultyTimestamps) > 0 && !AssertValidCriticalTimestamps(tx) {
		return errors.Wrap(VoteRejected, "tx is critical but has an invalid number of critical timestamps")
	}

	return nil
}

// AssertValidParentDepths asserts that the transaction has sane
// parents, and that we have the transactions parents in-store.
func AssertValidParentDepths(view *graph, tx *Transaction) bool {
	for _, parentID := range tx.ParentIDs {
		parent, stored := view.lookupTransaction(parentID)

		if !stored {
			return false
		}

		if parent.depth+sys.MaxEligibleParentsDepthDiff < tx.depth {
			return false
		}
	}

	return true
}

// Assert that the transaction has a sane timestamp, and that we
// have the transactions parents in-store.
func AssertValidTimestamp(view *graph, tx *Transaction) bool {
	visited := make(map[common.TransactionID]struct{})
	q := queue.New()

	for _, parentID := range tx.ParentIDs {
		if parent, stored := view.lookupTransaction(parentID); stored {
			q.PushBack(parent)
		} else {
			return false
		}

		visited[parentID] = struct{}{}
	}

	var timestamps []uint64

	for q.Len() > 0 {
		popped := q.PopFront().(*Transaction)

		timestamps = append(timestamps, popped.Timestamp)

		if popped.ID == view.Root().ID || len(timestamps) == sys.MedianTimestampNumAncestors {
			break
		}

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				parent, stored := view.lookupTransaction(parentID)

				if !stored {
					return false
				}

				q.PushBack(parent)
				visited[popped.ID] = struct{}{}
			}
		}
	}

	median := computeMedianTimestamp(timestamps)

	// Check that the transactions timestamp is within the range:
	//
	// TIMESTAMP ∈ (median(last 10 BFS-ordered transactions in terms of history), nodes current time + 2 hours]
	if tx.Timestamp <= median {
		return false
	}

	if tx.Timestamp > uint64(time.Duration(time.Now().Add(2*time.Hour).UnixNano())/time.Millisecond) {
		return false
	}

	return true
}

func AssertValidCriticalTimestamps(tx *Transaction) bool {
	if len(tx.DifficultyTimestamps) != computeCriticalTimestampWindowSize(tx.ViewID) {
		return false
	}

	// TODO(kenta): check if we have stored a log of the last 10 critical
	//  transactions timestamps to assert that the timestamps are the same.

	// Check that difficulty timestamps are in ascending order.
	for i := 1; i < len(tx.DifficultyTimestamps); i++ {
		if tx.DifficultyTimestamps[i] < tx.DifficultyTimestamps[i-1] {
			return false
		}
	}

	return true
}
