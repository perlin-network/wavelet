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

	if err := eddsa.Verify(tx.Creator[:], append([]byte{tx.Tag}, tx.Payload...), tx.CreatorSignature[:]); err != nil {
		return errors.New("tx has invalid creator signature")
	}

	cpy := *tx
	cpy.SenderSignature = common.ZeroSignature

	if err := eddsa.Verify(tx.Sender[:], cpy.Write(), tx.SenderSignature[:]); err != nil {
		return errors.New("tx has either invalid sender signature")
	}

	return nil
}

// AssertValidParentDepths asserts that the transaction has sane
// parents, and that we have the transactions parents in-store.
func AssertValidParentDepths(view *graph, tx *Transaction) error {
	for _, parentID := range tx.ParentIDs {
		parent, stored := view.lookupTransaction(parentID)

		if !stored {
			return errors.New("do not have tx parents in view-graph")
		}

		if parent.depth+sys.MaxEligibleParentsDepthDiff < tx.depth {
			return errors.New("tx parents exceeds max eligible parents depth diff")
		}
	}

	return nil
}

// Assert that the transaction has a sane timestamp, and that we
// have the transactions parents in-store.
func AssertValidTimestamp(view *graph, tx *Transaction) error {
	visited := make(map[common.TransactionID]struct{})
	q := queue.New()

	for _, parentID := range tx.ParentIDs {
		if parent, stored := view.lookupTransaction(parentID); stored {
			q.PushBack(parent)
		} else {
			return errors.New("do not have tx parents in view-graph")
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
					return errors.New("do not have tx ancestors in view-graph")
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
		return errors.Errorf("tx timestamp %d is lower than the median timestamp %d", tx.Timestamp, median)
	}

	if tx.Timestamp > uint64(time.Duration(time.Now().Add(2*time.Hour).UnixNano())/time.Millisecond) {
		return errors.Errorf("tx timestamp %d is greater than 2 hours from now", tx.Timestamp)
	}

	return nil
}

func AssertValidCriticalTimestamps(tx *Transaction) error {
	if size := computeCriticalTimestampWindowSize(tx.ViewID); len(tx.DifficultyTimestamps) != size {
		return errors.Errorf("expected tx to only have %d timestamp(s), but has %d timestamp(s)", size, len(tx.DifficultyTimestamps))
	}

	// TODO(kenta): check if we have stored a log of the last 10 critical
	//  transactions timestamps to assert that the timestamps are the same.

	// Check that difficulty timestamps are in ascending order.
	for i := 1; i < len(tx.DifficultyTimestamps); i++ {
		if tx.DifficultyTimestamps[i] < tx.DifficultyTimestamps[i-1] {
			return errors.New("tx critical timestamps are not in ascending order")
		}
	}

	return nil
}
