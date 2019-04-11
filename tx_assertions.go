package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"sort"
	"time"
)

func AssertValidTransaction(tx Transaction) error {
	if tx.ID == common.ZeroTransactionID {
		return errors.New("tx must not be empty")
	}

	if tx.Sender == common.ZeroAccountID || tx.Creator == common.ZeroAccountID {
		return errors.New("tx must have sender or creator")
	}

	if len(tx.ParentIDs) == 0 {
		return errors.New("tx must have parents")
	}

	// Check that parents are lexicographically sorted, are not itself, and are unique.
	set := make(map[common.TransactionID]struct{})

	for i := len(tx.ParentIDs) - 1; i > 0; i-- {
		if tx.ID == tx.ParentIDs[i] {
			return errors.New("tx must not include itself in its parents")
		}

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

	if tx.Tag > sys.TagStake {
		return errors.New("tx has an unknown tag")
	}

	if tx.Tag != sys.TagNop && len(tx.Payload) == 0 {
		return errors.New("tx must have payload if not a nop transaction")
	}

	if tx.Tag == sys.TagNop && len(tx.Payload) != 0 {
		return errors.New("tx must have no payload if is a nop transaction")
	}

	if err := eddsa.Verify(tx.Creator[:], append([]byte{tx.Tag}, tx.Payload...), tx.CreatorSignature[:]); err != nil {
		return errors.New("tx has invalid creator signature")
	}

	cpy := tx
	cpy.SenderSignature = common.ZeroSignature

	if err := eddsa.Verify(tx.Sender[:], cpy.Write(), tx.SenderSignature[:]); err != nil {
		return errors.New("tx has either invalid sender signature")
	}

	return nil
}

// AssertValidAncestry asserts that:
//
// 1) we have the transactions ancestral data in our view-graph.
// 2) the transaction has parents that are at a valid graph depth.
// 3) the transaction has a sane timestamp with respect to its ancestry.
func AssertValidAncestry(view *graph, tx Transaction) (missing []common.TransactionID, err error) {
	visited := make(map[common.TransactionID]struct{})
	q := queuePool.Get().(*queue.Queue)
	defer func() {
		q.Init()
		queuePool.Put(q)
	}()

	root := view.loadRoot()

	for _, parentID := range tx.ParentIDs {
		visited[parentID] = struct{}{}

		parent, stored := view.lookupTransaction(parentID)

		if !stored {
			missing = append(missing, parentID)
			continue
		}

		// Check if the depth of each parents is acceptable.
		if parent.depth+sys.MaxEligibleParentsDepthDiff < tx.depth {
			return missing, errors.New("tx parents exceeds max eligible parents depth diff")
		}

		q.PushBack(parent)
	}

	for _, rootParentID := range root.ParentIDs {
		visited[rootParentID] = struct{}{}
	}

	var timestamps []uint64

	for q.Len() > 0 && len(timestamps) < sys.MedianTimestampNumAncestors {
		popped := q.PopFront().(*Transaction)

		timestamps = append(timestamps, popped.Timestamp)

		for _, parentID := range popped.ParentIDs {
			if _, seen := visited[parentID]; !seen {
				visited[popped.ID] = struct{}{}

				parent, stored := view.lookupTransaction(parentID)

				if !stored {
					missing = append(missing, parentID)
					continue
				}

				q.PushBack(parent)

			}
		}
	}

	if len(missing) > 0 {
		return missing, errors.Wrapf(ErrMissingParents, "missing %d ancestor(s) in view-graph for tx %x", len(missing), tx.ID)
	}

	median := computeMedianTimestamp(timestamps)

	// TIMESTAMP ∈ (median(last 10 BFS-ordered transactions in terms of history), nodes current time + 2 hours]
	if tx.Timestamp <= median {
		return nil, errors.Errorf("tx timestamp %d is lower than the median timestamp %d", tx.Timestamp, median)
	}

	if tx.Timestamp > uint64(time.Duration(time.Now().Add(2*time.Hour).UnixNano())/time.Millisecond) {
		return nil, errors.Errorf("tx timestamp %d is greater than 2 hours from now", tx.Timestamp)
	}

	return nil, nil
}

func AssertInView(viewID uint64, kv store.KV, tx Transaction, critical bool) error {
	if critical {
		if tx.ViewID != viewID {
			return errors.Errorf("critical transaction was made for view ID %d, but our view ID is %d", tx.ViewID, viewID)
		}

		if tx.AccountsMerkleRoot == common.ZeroMerkleNodeID {
			return errors.New("critical transactions merkle root is expected to be not nil")
		}

		txCriticalTimestampsNum := len(tx.DifficultyTimestamps)
		if size := computeCriticalTimestampWindowSize(tx.ViewID); txCriticalTimestampsNum != size {
			return errors.Errorf("expected tx to have %d timestamp(s), but has %d timestamp(s)", size, txCriticalTimestampsNum)
		}

		// Check that difficulty timestamps are in ascending order.
		for i := 1; i < txCriticalTimestampsNum; i++ {
			if tx.DifficultyTimestamps[i] < tx.DifficultyTimestamps[i-1] {
				return errors.New("tx critical timestamps are not in ascending order")
			}
		}

		// Check that difficulty timestamps are same as stored ones
		savedTimestamps, err := ReadCriticalTimestamps(kv)
		if err != nil {
			return err
		}

		tsNumToCompare := len(savedTimestamps)
		if txCriticalTimestampsNum < tsNumToCompare {
			tsNumToCompare = txCriticalTimestampsNum
		}

		for i := 1; i <= tsNumToCompare; i++ {
			if savedTimestamps[len(savedTimestamps)-i] != tx.DifficultyTimestamps[txCriticalTimestampsNum-i] {
				return errors.New("tx critical timestamps differ from the stored ones")
			}
		}
	} else {
		if tx.AccountsMerkleRoot != common.ZeroMerkleNodeID {
			return errors.New("transactions merkle root is expected to be nil")
		}

		if len(tx.DifficultyTimestamps) > 0 {
			return errors.New("normal transactions are not expected to have difficulty timestamps")
		}

		if tx.ViewID < viewID {
			return errors.Errorf("transaction was made for view ID %d, but our view ID is %d", tx.ViewID, viewID)
		}
	}

	return nil
}
