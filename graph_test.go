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
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
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

	depthLimit := graph.height - 1 - sys.MaxDepthDiff

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
	tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.depthIndex[(graph.height-1)-(sys.MaxDepthDiff+2)][0])

	// An error should occur.
	assert.Error(t, graph.AddTransaction(tx))

	// Create a transaction at an eligible depth.
	tx = AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.FindEligibleParents()...)

	// No error should occur.
	assert.NoError(t, graph.AddTransaction(tx))
}

func TestGraph_UpdateRootDepth(t *testing.T) {
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
	graph.UpdateRootDepth(missingParent.Depth + sys.MaxDepthDiff + 1)

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

	tx := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagNop, nil), graph.depthIndex[(graph.height-1)-(sys.MaxDepthDiff+2)][0])

	tx.Depth += sys.MaxDepthDiff
	assert.True(t, errors.Cause(graph.validateTransactionParents(&tx)) == ErrDepthLimitExceeded)

	tx.Depth--
	assert.True(t, errors.Cause(graph.validateTransactionParents(&tx)) != ErrDepthLimitExceeded)
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
