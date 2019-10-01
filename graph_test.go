// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package wavelet

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewGraph(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))

	graph := NewGraph(WithRoot(tx))
	eligible := graph.FindEligibleParents()

	assert.Len(t, eligible, 1)
	assert.Equal(t, tx, *eligible[0])

	tx2 := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), eligible...)

	assert.NoError(t, graph.AddTransaction(tx2))
	assert.NotNil(t, graph.FindTransaction(tx2.ID))

	assert.Equal(t, graph.AddTransaction(tx2), ErrAlreadyExists)

	eligible = graph.FindEligibleParents()

	assert.Len(t, eligible, 1)
	assert.NotEqual(t, tx, *eligible[0])

	assert.Equal(t, graph.Height(), uint64(2))
}

func TestGraphAddTransaction(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	txs := []Transaction{root}
	for i := uint64(0); i < conf.GetMaxDepthDiff()+1; i++ {
		eligible := graph.FindEligibleParents()
		tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), eligible...)
		assert.NoError(t, graph.AddTransaction(tx))
		txs = append(txs, tx)
	}

	graph2 := NewGraph(WithRoot(txs[len(txs)-1]))
	assert.EqualError(t, graph2.AddTransaction(txs[0]), ErrDepthTooLow.Error())
}

func TestGraphValidateTransaction(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root), VerifySignatures())

	tests := []struct {
		Tx  func() Transaction
		Err string
	}{
		{func() Transaction { return NewTransaction(keys, sys.TagNop, nil) }, "tx must have an ID"},
		{
			func() Transaction {
				tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)
				tx.Sender = ZeroAccountID
				return tx
			},
			"tx must have sender associated to it",
		},
		{
			func() Transaction {
				tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)
				tx.Creator = ZeroAccountID
				return tx
			},
			"tx must have a creator associated to it",
		},
		{
			func() Transaction {
				k, _ := skademlia.NewKeys(1, 1)
				return AttachSenderToTransaction(k, NewTransaction(k, sys.TagNop, nil), []*Transaction{}...)
			},
			"transaction has no parents",
		},
		{
			func() Transaction {
				k, _ := skademlia.NewKeys(1, 1)

				parents := []*Transaction{}
				for i := 0; i < sys.MaxParentsPerTransaction+1; i++ {
					tx := NewTransaction(k, sys.TagNop, nil)
					parents = append(parents, &tx)
				}

				return AttachSenderToTransaction(k, NewTransaction(k, sys.TagNop, nil), parents...)
			},
			"tx has 33 parents, but tx may only have 32 parents at most",
		},
		{
			func() Transaction {
				tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)
				tx.ParentIDs = append(tx.ParentIDs, tx.ID)
				tx.ParentSeeds = append(tx.ParentSeeds, tx.Seed)
				tx.SenderSignature = edwards25519.Sign(keys.PrivateKey(), tx.Marshal())
				return tx
			},
			"tx must not include itself in its parents",
		},
		{
			func() Transaction {
				k, _ := skademlia.NewKeys(1, 1)

				// Add some unique transactions
				parents := make([]*Transaction, 6)
				parents[0] = &root
				for i := 0; i < 5; i++ {
					tx := AttachSenderToTransaction(k, NewTransaction(k, sys.TagNop, nil), []*Transaction{parents[i]}...)
					assert.NoError(t, graph.AddTransaction(tx))
					parents[i+1] = &tx
				}

				tx := AttachSenderToTransaction(keys, NewTransaction(k, sys.TagNop, nil), parents...)
				tx.ParentIDs[0], tx.ParentIDs[1] = tx.ParentIDs[1], tx.ParentIDs[0]
				tx.ParentSeeds[0], tx.ParentSeeds[1] = tx.ParentSeeds[1], tx.ParentSeeds[0]
				tx.SenderSignature = edwards25519.Sign(keys.PrivateKey(), tx.Marshal())
				tx.rehash()
				return tx
			},
			"tx must have lexicographically sorted parent ids",
		},
		{
			func() Transaction {
				parents := graph.FindEligibleParents()
				parents = append(parents, parents[0])
				return AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), parents...)
			},
			"tx must not have duplicate parent ids",
		},
		{
			func() Transaction {
				return AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagBatch+1, nil), graph.FindEligibleParents()...)
			},
			"tx has an unknown tag",
		},
		{
			func() Transaction {
				return AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, nil), graph.FindEligibleParents()...)
			},
			"tx must have payload if not a nop transaction",
		},
		{
			func() Transaction {
				payload := bytes.NewBuffer(nil)
				payload.Write([]byte("foobar"))
				return AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, payload.Bytes()), graph.FindEligibleParents()...)
			},
			"tx must have no payload if is a nop transaction",
		},
		{
			func() Transaction {
				k, _ := skademlia.NewKeys(1, 1)
				tx := AttachSenderToTransaction(keys, NewTransaction(k, sys.TagNop, nil), graph.FindEligibleParents()...)
				tx.CreatorSignature[0] = '0'
				return tx
			},
			"tx has invalid creator signature",
		},
		{
			func() Transaction {
				k, _ := skademlia.NewKeys(1, 1)
				tx := AttachSenderToTransaction(keys, NewTransaction(k, sys.TagNop, nil), graph.FindEligibleParents()...)
				tx.SenderSignature[0] = '0'
				return tx
			},
			"tx has invalid sender signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Err, func(t *testing.T) {
			assert.EqualError(t, graph.AddTransaction(tt.Tx()), fmt.Sprintf("failed to validate transaction: %s", tt.Err))
		})
	}
}

func TestGraphFuzz(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	var (
		payload [50]byte
		tx      Transaction
	)

	count := 1

	for i := 0; i < 500; i++ {
		_, err = rand.Read(payload[:])
		assert.NoError(t, err)

		tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, payload[:]), graph.FindEligibleParents()...)
		assert.NoError(t, graph.AddTransaction(tx))

		count++
	}

	assert.Len(t, graph.GetTransactionsByDepth(nil, nil), count)
	assert.Len(t, graph.GetTransactionsByDepth(&graph.depthIndex[1][0].Depth, nil), count-1)

	assert.Equal(t, graph.Len(), count)
	assert.Len(t, graph.transactions, count)
	assert.Len(t, graph.incomplete, 0)
	assert.Len(t, graph.Missing(), 0)

	var transactions []Transaction

	for _, tx := range graph.transactions {
		if tx.ID == root.ID {
			continue
		}

		transactions = append(transactions, *tx)
	}

	assert.Len(t, transactions, count-1)

	graph = NewGraph(WithRoot(root))

	rand.Shuffle(len(transactions), func(i, j int) {
		transactions[i], transactions[j] = transactions[j], transactions[i]
	})

	for _, tx := range transactions {
		_ = graph.AddTransaction(tx)
	}

	assert.Len(t, graph.transactions, count)
	assert.Len(t, graph.incomplete, 0)
	assert.Len(t, graph.Missing(), 0)

	// Assert that the graph is empty if we delete all of the graphs
	// roots progeny.

	graph.deleteProgeny(root.ID)

	assert.Len(t, graph.transactions, 0)
	assert.Len(t, graph.incomplete, 0)
	assert.Len(t, graph.Missing(), 0)
}

func TestGraphPruneBelowDepth(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	var (
		payload [50]byte
		tx      Transaction
	)

	count := 1
	pruneDepth := uint64(0)

	for i := 0; i < 500; i++ {
		if i == 500/2 {
			pruneDepth = graph.height
		}

		_, err = rand.Read(payload[:])
		assert.NoError(t, err)

		tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, payload[:]), graph.FindEligibleParents()...)
		assert.NoError(t, graph.AddTransaction(tx))
		count++
	}

	pruneCount := graph.PruneBelowDepth(pruneDepth)

	assert.Len(t, graph.transactions, count-pruneCount)
	assert.Len(t, graph.incomplete, 0)
	assert.Len(t, graph.missing, 0)

	for _, tx := range graph.transactions {
		assert.False(t, tx.Depth <= pruneDepth)
	}

	// Assert that pruning removes missing transactions below a certain depth.

	for depth := 1; depth <= 500; depth++ {
		var id TransactionID

		_, err = rand.Read(id[:])
		assert.NoError(t, err)

		graph.MarkTransactionAsMissing(id, uint64(depth))
	}

	assert.Len(t, graph.missing, 500)
	graph.PruneBelowDepth(250)
	assert.Len(t, graph.missing, 250)
	graph.PruneBelowDepth(500)
	assert.Len(t, graph.missing, 0)
}

func TestGraphUpdateRoot(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	var (
		payload [50]byte
		tx      Transaction
	)

	for i := 0; i < 50; i++ {
		_, err = rand.Read(payload[:])
		assert.NoError(t, err)

		tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, payload[:]), graph.FindEligibleParents()...)
		assert.NoError(t, graph.AddTransaction(tx))
	}

	// Assert that updating the root removes missing transactions and
	// their children below a certain depth.

	depthLimit := graph.height - 1 - conf.GetMaxDepthDiff()

	for depth := depthLimit - 30; depth < depthLimit+30; depth++ {
		var id TransactionID

		_, err = rand.Read(id[:])
		assert.NoError(t, err)

		graph.MarkTransactionAsMissing(id, uint64(depth))
	}

	numChildren := len(graph.children)

	// Update the root to a transaction at the top of the graph.
	graph.UpdateRootDepth((*graph.depthIndex[graph.height-1][0]).Depth)

	assert.Len(t, graph.missing, 30)
	assert.Len(t, graph.children, numChildren)

	// Create a transaction that is at an ineligible depth exceeding DEPTH_DIFF.
	tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.depthIndex[(graph.height-1)-(conf.GetMaxDepthDiff()+2)][0])

	// An error should occur.
	assert.Error(t, graph.AddTransaction(tx))

	// Create a transaction at an eligible depth.
	tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)

	// No error should occur.
	assert.NoError(t, graph.AddTransaction(tx))
}

func TestGraphUpdateRootDepth(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	missingParent := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), &root)

	var (
		payload [50]byte
		tx      Transaction
	)

	num := 50
	for i := 0; i < num; i++ {
		_, err = rand.Read(payload[:])
		assert.NoError(t, err)

		var parent *Transaction
		if i == 0 {
			parent = &missingParent
		} else {
			parent = &tx
		}

		tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, payload[:]), parent)
		_ = graph.AddTransaction(tx)
	}

	assert.Equal(t, num+1, len(graph.transactions))
	assert.Equal(t, 1, len(graph.missing))
	assert.Equal(t, num, len(graph.incomplete))

	// ensure updating root will delete missing transactions after some threshold and complete it's descendants
	graph.UpdateRootDepth(missingParent.Depth + conf.GetMaxDepthDiff() + 1)

	assert.Equal(t, num+1, len(graph.transactions))
	assert.Equal(t, 0, len(graph.missing))
	assert.Equal(t, 0, len(graph.incomplete))
}

func TestGraphValidateTransactionParents(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	for i := 0; i < 50; i++ {
		var depth []Transaction

		for i := 0; i < rand.Intn(sys.MaxParentsPerTransaction)+1; i++ {
			var payload [50]byte

			_, err = rand.Read(payload[:])
			assert.NoError(t, err)

			depth = append(depth, AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, payload[:]), graph.FindEligibleParents()...))
		}

		for _, tx := range depth {
			assert.NoError(t, graph.AddTransaction(tx))
		}
	}

	tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.depthIndex[(graph.height-1)-(conf.GetMaxDepthDiff()+2)][0])
	assert.NoError(t, graph.validateTransactionParents(&tx))

	assert.Equal(t, len(tx.ParentIDs), len(tx.ParentSeeds))

	parentSeed := tx.ParentSeeds[0]
	tx.ParentSeeds[0] = [32]byte{}
	assert.Error(t, graph.validateTransactionParents(&tx))
	tx.ParentSeeds[0] = parentSeed

	tx.Depth += conf.GetMaxDepthDiff()
	assert.True(t, errors.Cause(graph.validateTransactionParents(&tx)) == ErrParentDepthLimitExceeded)

	tx.Depth--
	assert.True(t, errors.Cause(graph.validateTransactionParents(&tx)) != ErrParentDepthLimitExceeded)
}

func TestGraphFindEligibleCritical(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	// Go through a range of difficulties, and check if we can always
	// find the eligible critical transaction.

	for difficulty := byte(2); difficulty < 8; difficulty++ {
		eligible := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)

		for {
			if eligible.IsCritical(difficulty) {
				break
			}

			sender, err := skademlia.NewKeys(1, 1)
			assert.NoError(t, err)

			eligible = AttachSenderToTransaction(sender, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)
		}

		assert.NoError(t, graph.AddTransaction(eligible))
		assert.Equal(t, *graph.FindEligibleCritical(difficulty), eligible)

		root = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
		graph = NewGraph(WithRoot(root))
	}
}

func TestGraphFindEligibleCriticalInBigGraph(t *testing.T) {
	t.Parallel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	root := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil))
	graph := NewGraph(WithRoot(root))

	difficulty := byte(8)

	var eligible Transaction

	for i := 0; i < 500; i++ {
		if i == 500/3 { // Prune away any eligible critical transactions a third through the graph.
			graph.UpdateRootDepth(graph.height - 1)
		}

		if i == 500/2 { // Create an eligible critical transaction in the middle of the graph.
			eligible = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)

			for {
				if eligible.IsCritical(difficulty) {
					break
				}

				sender, err := skademlia.NewKeys(1, 1)
				assert.NoError(t, err)

				eligible = AttachSenderToTransaction(sender, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)
			}

			assert.NoError(t, graph.AddTransaction(eligible))
		}

		var depth []Transaction

		for i := 0; i < rand.Intn(sys.MaxParentsPerTransaction)+1; i++ {
			var payload [50]byte

			_, err = rand.Read(payload[:])
			assert.NoError(t, err)

			tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, payload[:]), graph.FindEligibleParents()...)

			for { // Be sure we never create a transaction with the difficulty we set.
				if !tx.IsCritical(difficulty) {
					break
				}

				sender, err := skademlia.NewKeys(1, 1)
				assert.NoError(t, err)

				tx = AttachSenderToTransaction(sender, NewTransaction(keys, sys.TagTransfer, payload[:]), graph.FindEligibleParents()...)
			}

			depth = append(depth, tx)
		}

		for _, tx := range depth {
			assert.NoError(t, graph.AddTransaction(tx))
		}
	}

	assert.Equal(t, *graph.FindEligibleCritical(difficulty), eligible)
}
