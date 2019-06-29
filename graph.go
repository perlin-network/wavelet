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
	"encoding/hex"
	"github.com/google/btree"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"sort"
	"sync"
)

type GraphOption func(*Graph)

func WithRoot(root Transaction) GraphOption {
	return func(graph *Graph) {
		if graph.indexer != nil {
			graph.indexer.Index(hex.EncodeToString(root.ID[:]))
		}

		graph.UpdateRoot(root)
	}
}

func WithMetrics(metrics *Metrics) GraphOption {
	return func(graph *Graph) {
		graph.metrics = metrics
	}
}

func WithIndexer(indexer *Indexer) GraphOption {
	return func(graph *Graph) {
		graph.indexer = indexer
	}
}

func VerifySignatures() GraphOption {
	return func(graph *Graph) {
		graph.verifySignatures = true
	}
}

type sortByDepthTX Transaction

func (a *sortByDepthTX) Less(b btree.Item) bool {
	return a.Depth < b.(*sortByDepthTX).Depth
}

type sortBySeedTX Transaction

func (a *sortBySeedTX) Less(b btree.Item) bool {
	if a.Depth == b.(*sortBySeedTX).Depth {
		return a.SeedLen > b.(*sortBySeedTX).SeedLen
	}

	return a.Depth < b.(*sortBySeedTX).Depth
}

var (
	ErrMissingParents     = errors.New("parents for transaction are not in graph")
	ErrAlreadyExists      = errors.New("transaction already exists in the graph")
	ErrDepthLimitExceeded = errors.New("transactions parents exceed depth limit")
)

type Graph struct {
	sync.RWMutex

	metrics *Metrics
	indexer *Indexer

	transactions map[TransactionID]*Transaction    // All transactions. Includes incomplete transactions.
	children     map[TransactionID][]TransactionID // Children of transactions. Includes incomplete/missing transactions.

	missing    map[TransactionID]uint64   // Transactions that we are missing. Maps to depth of child of missing transaction.
	incomplete map[TransactionID]struct{} // Transactions that don't have all parents available.

	eligibleIndex *btree.BTree              // Transactions that are eligible to be parent transactions.
	seedIndex     *btree.BTree              // Indexes transactions by the number of zero bits prefixed of BLAKE2b(Sender || ParentIDs).
	depthIndex    map[uint64][]*Transaction // Indexes transactions by their depth.

	height    uint64 // Height of the graph.
	rootDepth uint64 // Depth of the graphs root.

	verifySignatures bool
}

func NewGraph(opts ...GraphOption) *Graph {
	g := &Graph{
		transactions: make(map[TransactionID]*Transaction),
		children:     make(map[TransactionID][]TransactionID),

		missing:    make(map[TransactionID]uint64),
		incomplete: make(map[TransactionID]struct{}),

		eligibleIndex: btree.New(32),
		seedIndex:     btree.New(32),
		depthIndex:    make(map[uint64][]*Transaction),
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// AddTransaction adds sufficiently valid transactions with a strongly connected ancestry
// to the graph, and otherwise buffers incomplete transactions, or otherwise rejects
// invalid transactions.
func (g *Graph) AddTransaction(tx Transaction) error {
	g.Lock()
	defer g.Unlock()

	if _, exists := g.transactions[tx.ID]; exists {
		return ErrAlreadyExists
	}

	if g.rootDepth > sys.MaxDepthDiff+tx.Depth {
		return errors.Errorf("transactions depth is too low compared to root: root depth is %d, but tx depth is %d", g.rootDepth, tx.Depth)
	}

	if err := g.validateTransaction(tx); err != nil {
		return errors.Wrap(err, "failed to validate transaction")
	}

	ptr := &tx

	g.transactions[tx.ID] = ptr
	delete(g.missing, tx.ID)

	parentsMissing := false

	// Do not consider transactions below root.depth by exactly DEPTH_DIFF to be incomplete
	// at all. Permit them to have incomplete parent histories.

	if g.rootDepth != uint64(sys.MaxParentsPerTransaction)+tx.Depth {
		for _, parentID := range tx.ParentIDs {
			if _, stored := g.transactions[parentID]; !stored {
				parentsMissing = true

				if _, recorded := g.missing[parentID]; !recorded {
					g.missing[parentID] = tx.Depth
				}
			}

			if _, incomplete := g.incomplete[parentID]; incomplete {
				parentsMissing = true
			}

			g.children[parentID] = append(g.children[parentID], tx.ID)
		}
	}

	if parentsMissing {
		g.incomplete[tx.ID] = struct{}{}

		return ErrMissingParents
	}

	return g.updateGraph(ptr)
}

// MarkTransactionAsMissing marks a transaction at some given depth to be
// missing.
func (g *Graph) MarkTransactionAsMissing(id TransactionID, depth uint64) {
	g.Lock()
	if g.rootDepth <= sys.MaxDepthDiff+depth {
		g.missing[id] = depth
	}
	g.Unlock()
}

// UpdateRoot forcefully adds a root transaction to the graph, and updates all
// relevant graph indices as a result of setting a new root with its new depth.
func (g *Graph) UpdateRoot(root Transaction) {
	ptr := &root

	g.Lock()

	g.depthIndex[root.Depth] = append(g.depthIndex[root.Depth], ptr)
	g.eligibleIndex.ReplaceOrInsert((*sortByDepthTX)(ptr))

	g.transactions[root.ID] = ptr

	g.height = root.Depth + 1

	g.Unlock()

	g.UpdateRootDepth(root.Depth)
}

// UpdateRootDepth updates the root depth of the graph to disallow new transactions
// from being added to the graph whose depth is less than root depth by at most
// DEPTH_DIFF. It additionally clears away any missing transactions that are at
// a depth below the root depth by more than DEPTH_DIFF.
func (g *Graph) UpdateRootDepth(rootDepth uint64) {
	var pendingDepth []*sortByDepthTX
	var pendingSeed []*sortBySeedTX

	g.Lock()

	g.rootDepth = rootDepth

	for id, depth := range g.missing {
		if rootDepth <= sys.MaxDepthDiff+depth {
			continue
		}

		delete(g.children, id)
		delete(g.missing, id)
	}

	g.eligibleIndex.Ascend(func(i btree.Item) bool {
		if rootDepth <= i.(*sortByDepthTX).Depth {
			return true
		}

		pendingDepth = append(pendingDepth, i.(*sortByDepthTX))

		return true
	})

	g.seedIndex.Ascend(func(i btree.Item) bool {
		if rootDepth < i.(*sortBySeedTX).Depth {
			return true
		}

		pendingSeed = append(pendingSeed, i.(*sortBySeedTX))

		return true
	})

	for _, i := range pendingDepth {
		g.eligibleIndex.Delete(i)
	}

	for _, i := range pendingSeed {
		g.seedIndex.Delete(i)
	}

	g.Unlock()
}

// PruneBelowDepth prunes all transactions and their indices that has a depth
// equal to or less than targetDepth.
func (g *Graph) PruneBelowDepth(targetDepth uint64) int {
	count := 0

	g.Lock()

	for depth := range g.depthIndex {
		if depth > targetDepth {
			continue
		}

		for _, tx := range g.depthIndex[depth] {
			count += tx.LogicalUnits()

			delete(g.transactions, tx.ID)
			delete(g.children, tx.ID)

			delete(g.missing, tx.ID)
			delete(g.incomplete, tx.ID)

			g.eligibleIndex.Delete((*sortByDepthTX)(tx))
			g.seedIndex.Delete((*sortBySeedTX)(tx))

			if g.indexer != nil {
				g.indexer.Index(hex.EncodeToString(tx.ID[:]))
			}
		}

		delete(g.depthIndex, depth)
	}

	for id, depth := range g.missing {
		if depth > targetDepth {
			continue
		}

		delete(g.children, id)
		delete(g.missing, id)
	}

	g.Unlock()

	return count
}

// FindEligibleParents provides a set of transactions suited to be eligible
// parents. We consider eligible parents to be transactions closest to the
// graphs frontier by DEPTH_DIFF that have no children, such that they are
// leaf nodes of the graph.
func (g *Graph) FindEligibleParents() []*Transaction {
	var eligibleParents []*Transaction
	var pending []*sortByDepthTX

	g.Lock()

	g.eligibleIndex.Descend(func(i btree.Item) bool {
		eligibleParent := i.(*sortByDepthTX)

		if g.height-1 >= sys.MaxDepthDiff+eligibleParent.Depth {
			pending = append(pending, eligibleParent)
			return true
		}

		// Recall that a transaction that may have children that are incomplete. So,
		// it is good to do a sanity check to see if the transaction is a leaf node
		// by checking if it has no children, or if it only comprises of incomplete
		// or missing transactions.

		if children, exists := g.children[eligibleParent.ID]; exists {
			for _, childID := range children {
				_, missing := g.missing[childID]
				_, incomplete := g.incomplete[childID]

				if !missing && !incomplete {
					pending = append(pending, eligibleParent)
					return true
				}
			}
		}

		eligibleParents = append(eligibleParents, (*Transaction)(eligibleParent))

		return len(eligibleParents) != sys.MaxParentsPerTransaction
	})

	for _, i := range pending {
		g.eligibleIndex.Delete(i)
	}

	g.Unlock()

	return eligibleParents
}

// FindEligibleCritical looks through all transactions in the current
// round, and returns any one whose number of zero bits prefixed of
// its seed is >= difficulty.
func (g *Graph) FindEligibleCritical(difficulty byte) *Transaction {
	var pending []*sortBySeedTX
	var critical *Transaction

	g.Lock()

	g.seedIndex.Ascend(func(i btree.Item) bool {
		tx := i.(*sortBySeedTX)

		if tx.Depth <= g.rootDepth {
			pending = append(pending, tx)
			return true
		}

		if !(*Transaction)(tx).IsCritical(difficulty) {
			pending = append(pending, tx)
			return true
		}

		critical = (*Transaction)(tx)

		return false
	})

	for _, i := range pending {
		g.seedIndex.Delete(i)
	}

	g.Unlock()

	return critical
}

// GetTransactionsByDepth returns all transactions in graph whose depth is
// between [start, end].
func (g *Graph) GetTransactionsByDepth(start *uint64, end *uint64) []*Transaction {
	var transactions []*Transaction

	g.RLock()
	for depth, index := range g.depthIndex {
		if (start != nil && depth < *start) || (end != nil && depth > *end) {
			continue
		}

		transactions = append(transactions, index...)
	}
	g.RUnlock()

	return transactions
}

func (g *Graph) Missing() []TransactionID {
	g.RLock()
	missing := make([]TransactionID, 0, len(g.missing))

	for id := range g.missing {
		missing = append(missing, id)
	}

	sort.Slice(missing, func(i, j int) bool {
		return g.missing[missing[i]] < g.missing[missing[j]]
	})

	g.RUnlock()

	return missing
}

func (g *Graph) ListTransactions(offset, limit uint64, sender, creator AccountID) (transactions []*Transaction) {
	g.RLock()
	defer g.RUnlock()

	for _, tx := range g.transactions {
		if (sender == ZeroAccountID && creator == ZeroAccountID) || (sender != ZeroAccountID && tx.Sender == sender) || (creator != ZeroAccountID && tx.Creator == creator) {
			transactions = append(transactions, tx)
		}
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Depth > transactions[j].Depth
	})

	if offset != 0 || limit != 0 {
		if offset >= uint64(len(transactions)) {
			return nil
		}

		if offset+limit > uint64(len(transactions)) {
			limit = uint64(len(transactions)) - offset
		}

		transactions = transactions[offset : offset+limit]
	}

	return
}

// FindTransaction returns transaction with id from graph, and nil otherwise.
func (g *Graph) FindTransaction(id TransactionID) *Transaction {
	g.RLock()
	tx := g.transactions[id]
	g.RUnlock()

	return tx
}

// Height returns the height of the graph.
func (g *Graph) Height() uint64 {
	g.RLock()
	height := g.height
	g.RUnlock()

	return height
}

// Len returns the number of transactions in the graph.
func (g *Graph) Len() int {
	g.RLock()
	num := len(g.transactions)
	g.RUnlock()

	return num
}

// MissingLen returns the number of known missing transactions of the graph.
func (g *Graph) MissingLen() int {
	g.RLock()
	num := len(g.missing)
	g.RUnlock()

	return num
}

// GetTransactionsByDepth returns the number of transactions in graph whose depth is
// between [start, end].
func (g *Graph) DepthLen(start *uint64, end *uint64) int {
	count := 0

	g.RLock()
	for depth, index := range g.depthIndex {
		if (start != nil && depth < *start) || (end != nil && depth > *end) {
			continue
		}

		count += len(index)
	}
	g.RUnlock()

	return count
}

// RootDepth returns the current depth of the root transaction of the graph.
func (g *Graph) RootDepth() uint64 {
	g.RLock()
	rootDepth := g.rootDepth
	g.RUnlock()

	return rootDepth
}

func (g *Graph) updateGraph(tx *Transaction) error {
	if err := g.validateTransactionParents(tx); err != nil {
		g.deleteProgeny(tx.ID)

		return err
	}

	if g.height < tx.Depth+1 { // Update graph height.
		g.height = tx.Depth + 1
	}

	g.eligibleIndex.ReplaceOrInsert((*sortByDepthTX)(tx))       // Index transaction to be eligible.
	g.seedIndex.ReplaceOrInsert((*sortBySeedTX)(tx))            // Index transaction based on num prefixed zero bits of seed.
	g.depthIndex[tx.Depth] = append(g.depthIndex[tx.Depth], tx) // Index transaction by depth.

	if g.metrics != nil {
		g.metrics.receivedTX.Mark(int64(tx.LogicalUnits()))
	}

	for _, childID := range g.children[tx.ID] {
		if _, incomplete := g.incomplete[childID]; !incomplete {
			continue
		}

		child, exists := g.transactions[childID]
		if !exists {
			continue
		}

		complete := true // Complete if parents are complete, and parent transaction contents exist in graph.

		for _, parentID := range child.ParentIDs {
			if _, incomplete := g.incomplete[parentID]; incomplete {
				complete = false
				break
			}

			if _, exists := g.transactions[parentID]; !exists {
				complete = false
				break
			}
		}

		if complete {
			delete(g.incomplete, childID)

			if g.indexer != nil {
				g.indexer.Remove(hex.EncodeToString(tx.ID[:]))
			}

			if err := g.updateGraph(child); err != nil {
				continue
			}
		}
	}

	return nil
}

func (g *Graph) deleteProgeny(id TransactionID) {
	children := g.children[id]

	tx, exists := g.transactions[id]
	if exists {
		g.eligibleIndex.Delete((*sortByDepthTX)(tx))
		g.seedIndex.Delete((*sortBySeedTX)(tx))

		if len(g.depthIndex[tx.Depth]) > 0 {
			slice := g.depthIndex[tx.Depth][:0]

			for _, it := range g.depthIndex[tx.Depth][:0] {
				if it.ID == tx.ID {
					continue
				}

				slice = append(slice, it)
			}

			g.depthIndex[tx.Depth] = slice
		}
	}

	delete(g.transactions, id)
	delete(g.children, id)

	delete(g.missing, id)
	delete(g.incomplete, id)

	for _, childID := range children {
		g.deleteProgeny(childID)
	}
}

func (g *Graph) validateTransaction(tx Transaction) error {
	if tx.ID == ZeroTransactionID {
		return errors.New("tx must have an ID")
	}

	if tx.Sender == ZeroAccountID {
		return errors.New("tx must have sender associated to it")
	}

	if tx.Creator == ZeroAccountID {
		return errors.New("tx must have a creator associated to it")
	}

	if len(tx.ParentIDs) == 0 {
		return errors.New("transaction has no parents")
	}

	if len(tx.ParentIDs) > sys.MaxParentsPerTransaction {
		return errors.Errorf("tx has %d parents, but tx may only have %d parents at most", len(tx.ParentIDs), sys.MaxParentsPerTransaction)
	}

	// Check that parents are lexicographically sorted, are not itself, and are unique.
	set := make(map[TransactionID]struct{}, len(tx.ParentIDs))

	for i := len(tx.ParentIDs) - 1; i > 0; i-- {
		if tx.ID == tx.ParentIDs[i] {
			return errors.New("tx must not include itself in its parents")
		}

		if bytes.Compare(tx.ParentIDs[i-1][:], tx.ParentIDs[i][:]) > 0 {
			return errors.New("tx must have lexicographically sorted parent ids")
		}

		if _, duplicate := set[tx.ParentIDs[i]]; duplicate {
			return errors.New("tx must not have duplicate parent ids")
		}

		set[tx.ParentIDs[i]] = struct{}{}
	}

	if tx.Tag > sys.TagBatch {
		return errors.New("tx has an unknown tag")
	}

	if tx.Tag != sys.TagNop && len(tx.Payload) == 0 {
		return errors.New("tx must have payload if not a nop transaction")
	}

	if tx.Tag == sys.TagNop && len(tx.Payload) != 0 {
		return errors.New("tx must have no payload if is a nop transaction")
	}

	if g.verifySignatures {
		var nonce [8]byte // TODO(kenta): nonce

		if tx.Sender != tx.Creator {
			if !edwards25519.Verify(tx.Creator, append(nonce[:], append([]byte{tx.Tag}, tx.Payload...)...), tx.CreatorSignature) {
				return errors.New("tx has invalid creator signature")
			}
		}

		cpy := tx
		cpy.SenderSignature = ZeroSignature

		if !edwards25519.Verify(tx.Sender, cpy.Marshal(), tx.SenderSignature) {
			return errors.New("tx has invalid sender signature")
		}
	}

	return nil
}

func (g *Graph) validateTransactionParents(tx *Transaction) error {
	// Do not consider transactions below root.depth by exactly DEPTH_DIFF to be incomplete
	// at all. Permit them to have incomplete parent histories.

	if g.rootDepth == uint64(sys.MaxDepthDiff)+tx.Depth {
		return nil
	}

	var maxDepth uint64

	for _, parentID := range tx.ParentIDs {
		parent, exists := g.transactions[parentID]

		if !exists {
			return errors.New("parent not stored in graph")
		}

		if tx.Depth > sys.MaxDepthDiff+parent.Depth { // Check if the depth of each parents is acceptable.
			return errors.Wrapf(ErrDepthLimitExceeded, "tx parent has ineligible depth: parents depth is %d, but tx depth is %d", parent.Depth, tx.Depth)
		}

		if maxDepth < parent.Depth { // Update max depth witnessed from parents.
			maxDepth = parent.Depth
		}
	}

	maxDepth++

	if tx.Depth != maxDepth {
		return errors.Errorf("transactions depth is invalid: expected depth to be %d but got %d", maxDepth, tx.Depth)
	}

	return nil
}
