package wavelet

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"github.com/fortytw2/leaktest"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"sync"
	"testing"
)

func signalWhenComplete(wg *sync.WaitGroup, l *Ledger, fn transition) {
	fn(l)
	wg.Done()
}

func call(wg *sync.WaitGroup, fn func() error) error {
	defer wg.Done()
	return fn()
}

func formPayload(recipient string, amount uint64) ([]byte, error) {
	b, err := hex.DecodeString(recipient)
	if err != nil {
		return nil, err
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)

	payload.Write(b)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])

	return payload.Bytes(), nil
}

func TestStopNoLeaks(t *testing.T) {
	defer leaktest.Check(t)()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	l := NewLedger(ctx, keys, store.NewInmem())
	l.Stop()
}

func TestKill(t *testing.T) {
	defer leaktest.Check(t)()

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	var wg sync.WaitGroup

	// Test if we can gracefully stop the ledger while it is gossiping.
	l := NewLedger(ctx, keys, store.NewInmem())

	wg.Add(1)
	go signalWhenComplete(&wg, l, gossiping)
	l.Stop()
	wg.Wait()

	// Test if we can gracefully stop the ledger while it is querying.
	l = NewLedger(ctx, keys, store.NewInmem())
	l.cr.Prefer(Transaction{})

	wg.Add(1)
	go signalWhenComplete(&wg, l, querying)
	l.Stop()
	wg.Wait()
}

func TestGossipOutTransaction(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())
	defer l.Stop()

	go gossiping(l)

	// Create a dummy broadcast event.
	tx, err := NewTransaction(l.keys, sys.TagTransfer, []byte("lorem ipsum"))
	assert.NoError(t, err)

	evt := EventBroadcast{
		Tag:       tx.Tag,
		Payload:   tx.Payload,
		Creator:   tx.Creator,
		Signature: tx.CreatorSignature,
		Result:    make(chan Transaction, 1),
		Error:     make(chan error, 1),
	}

	// Queue up the transaction we want to broadcast.
	l.BroadcastQueue <- evt

	// Collect the gossip that the ledger wanted to send out.
	out := <-l.GossipOut

	// Signal that the gossip was sent out successfully.
	out.Result <- []VoteGossip{{Ok: true}}

	// Assert no errors
	select {
	case err := <-evt.Error:
		assert.EqualError(t, err, "tx creator does not have enough PERLs to pay for fees")
	case <-evt.Result:
		assert.FailNow(t, "Tx expected to fail")
	}

	// Assert that the transactions are the same.
	assert.Equal(t, evt.Payload, out.TX.Payload)

	// Assert that the transaction has a sender attached.
	assert.NotZero(t, out.TX.Timestamp)
	assert.NotEmpty(t, out.TX.ParentIDs)
	assert.NotEmpty(t, out.TX.Sender)
	assert.NotEmpty(t, out.TX.SenderSignature)
}

func TestTransitionFromGossipingToQuerying(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())
	defer l.Stop()

	preferred, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)

	preferred.rehash()

	l.cr.Prefer(preferred)

	// Create a dummy broadcast event.
	p, err := formPayload("400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405", 10)
	if !assert.NoError(t, err) {
		return
	}

	tx, err := NewTransaction(l.keys, sys.TagTransfer, p)
	assert.NoError(t, err)

	snapshot := l.a.snapshot()
	WriteAccountBalance(snapshot, tx.Creator, 1000)
	if !assert.NoError(t, l.a.commit(snapshot)) {
		return
	}

	evt := EventBroadcast{
		Tag:       tx.Tag,
		Payload:   tx.Payload,
		Creator:   tx.Creator,
		Signature: tx.CreatorSignature,
		Result:    make(chan Transaction, 1),
		Error:     make(chan error, 1),
	}

	// Queue up the transaction we want to broadcast.
	l.BroadcastQueue <- evt

	// Run a single iteration of gossiping with a preferred transaction.
	next := make(chan error)
	go func() { next <- gossip(l)(nil) }()
	defer close(next)

	// Collect the gossip that the ledger wanted to send out.
	out := <-l.GossipOut

	// Signal that the gossip was sent out successfully.
	out.Result <- []VoteGossip{{Ok: true}}

	// Assert no errors.
	select {
	case err := <-evt.Error:
		assert.NoError(t, err)
	case <-evt.Result:
	}

	// Assert that we received a signal to transition to querying.
	assert.Equal(t, ErrPreferredSelected, <-next)
}

func TestEnsureGossipReturnsNetworkErrors(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())
	defer l.Stop()

	// Create a dummy broadcast event.
	tx, err := NewTransaction(l.keys, sys.TagTransfer, []byte("lorem ipsum"))
	assert.NoError(t, err)

	evt := EventBroadcast{
		Tag:       tx.Tag,
		Payload:   tx.Payload,
		Creator:   tx.Creator,
		Signature: tx.CreatorSignature,
		Result:    make(chan Transaction, 1),
		Error:     make(chan error, 1),
	}

	// Queue up the transaction we want to broadcast.
	l.BroadcastQueue <- evt

	// Run a single iteration of gossiping.
	next := make(chan error)
	go func() { next <- gossip(l)(nil) }()
	defer close(next)

	// Collect the gossip that the ledger wanted to send out.
	out := <-l.GossipOut

	// Signal that the gossip was unsuccessful.
	out.Error <- errors.New("failed")

	select {
	case err := <-evt.Error:
		assert.EqualError(t, err, "got an error gossiping transaction out: failed")
	case <-evt.Result:
		assert.FailNow(t, "Tx expected to fail")
	}

	// Assert that we received no signal.
	assert.Equal(t, nil, <-next)
}

// Note: This test will take about (2 * timeout) seconds because of the timeout tests.
func TestQuery(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	preferred, err := NewTransaction(l.keys, sys.TagNop, nil)
	if !assert.NoError(t, err) {
		return
	}

	preferred.ViewID = l.ViewID()
	preferred.rehash()

	l.cr.Prefer(preferred)
	l.cr.decided = true

	stop := make(chan struct{})
	state := new(stateQuerying)

	query := func() error {
		return query(l, state)(stop)
	}

	var wg sync.WaitGroup
	var evt EventQuery

	// Test query empty votes.
	wg.Add(1)
	go func() {
		assert.NoError(t, call(&wg, query))
	}()

	evt = <-l.QueryOut
	evt.Result <- []VoteQuery{}
	wg.Wait()

	// Test query.
	wg.Add(2)
	go func() {
		assert.Equal(t, ErrConsensusRoundFinished, call(&wg, query))

		// Test preferred nil.
		assert.Equal(t, ErrConsensusRoundFinished, call(&wg, query))
	}()
	evt = <-l.QueryOut
	evt.Result <- []VoteQuery{
		{
			Voter: common.AccountID{},
			Preferred: Transaction{
				ID:     preferred.ID,
				ViewID: 1,
			},
		},
	}
	wg.Wait()

	// Re-set the preferred.
	preferred, err = NewTransaction(l.keys, sys.TagNop, nil)
	if !assert.NoError(t, err) {

	}

	preferred.ViewID = l.ViewID()
	preferred.rehash()
	l.cr.Prefer(preferred)

	// Test query error.
	wg.Add(1)
	evtError := errors.New("query error")
	go func() {
		err := call(&wg, query)
		assert.Equal(t, evtError, errors.Cause(err))
	}()

	evt = <-l.QueryOut
	evt.Error <- evtError
	wg.Wait()

	// Test the second select: stop
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrStopped, call(&wg, query))
	}()
	evt = <-l.QueryOut
	close(stop)
	wg.Wait()
	stop = make(chan struct{})

	// Test the second select: kill.
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrStopped, call(&wg, query))
	}()
	evt = <-l.QueryOut
	l.Stop()
	wg.Wait()
	l.kill = make(chan struct{})

	// Test the second select: timeout.
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrTimeout, errors.Cause(call(&wg, query)))
	}()
	evt = <-l.QueryOut
	wg.Wait()

	// Test the first select.

	// Disable the queryOut case.
	l.queryOut = nil

	// Test the first select: timeout.
	err = query()
	assert.Equal(t, ErrTimeout, errors.Cause(err))

	// Test the first select: stop.
	close(stop)
	assert.Equal(t, ErrStopped, query())
	stop = make(chan struct{})

	// Test the first select: kill.
	l.Stop()
	assert.Equal(t, ErrStopped, query())
}

func TestListenForQueries(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	root, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)

	root.ViewID = 1

	root.rehash()

	l.v.saveRoot(&root)

	stop := make(chan struct{})
	listenForQueries := func() error {
		return listenForQueries(l)(stop)
	}

	// Test root response.

	evt := EventIncomingQuery{
		TX: Transaction{
			ViewID: root.ViewID,
		},
		Response: make(chan *Transaction, 1),
		Error:    make(chan error, 1),
	}
	l.QueryIn <- evt
	assert.Error(t, ErrConsensusRoundFinished, listenForQueries())
	tx := <-evt.Response
	assert.Equal(t, l.v.loadRoot().ID, tx.ID)

	// Test nil response.

	evt = EventIncomingQuery{
		TX:       Transaction{},
		Response: make(chan *Transaction, 1),
		Error:    make(chan error, 1),
	}
	l.QueryIn <- evt
	assert.Error(t, ErrConsensusRoundFinished, listenForQueries())
	tx = <-evt.Response
	assert.Nil(t, tx)

	// Test preferred response.

	preferred, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)
	preferred.rehash()
	l.cr.Prefer(preferred)

	evt = EventIncomingQuery{
		Response: make(chan *Transaction, 1),
		Error:    make(chan error, 1),
	}

	l.QueryIn <- evt
	assert.NoError(t, listenForQueries())
	tx = <-evt.Response
	assert.Equal(t, preferred.ID, tx.ID)

	// Test stop.

	close(stop)
	assert.Equal(t, ErrStopped, listenForQueries())

	stop = make(chan struct{})

	// Test kill.

	l.Stop()
	assert.Equal(t, ErrStopped, listenForQueries())
}

func TestListenForSyncDiffChunks(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	stop := make(chan struct{})
	listenForSyncDiffChunks := func() error {
		return listenForSyncDiffChunks(l)(stop)
	}

	var chunkHash [blake2b.Size256]byte
	_, err = rand.Read(chunkHash[:])
	assert.NoError(t, err)

	var evt EventIncomingSyncDiff

	// Test ChunkHash not found
	evt = EventIncomingSyncDiff{
		ChunkHash: chunkHash,
		Response:  make(chan []byte, 1),
	}

	l.SyncDiffIn <- evt
	assert.NoError(t, listenForSyncDiffChunks())
	// check the response should be nil
	assert.Nil(t, <-evt.Response)

	// Test ChunkHash found

	// Save the ChunkHash
	var value = []byte("chunk")
	l.cacheDiffChunks.put(chunkHash, value)

	evt = EventIncomingSyncDiff{
		ChunkHash: chunkHash,
		Response:  make(chan []byte, 1),
	}
	l.SyncDiffIn <- evt
	assert.NoError(t, listenForSyncDiffChunks())
	// check the response
	assert.Equal(t, value, <-evt.Response)

	// Test stop

	close(stop)
	assert.Equal(t, ErrStopped, listenForSyncDiffChunks())

	stop = make(chan struct{})

	// Test kill.

	l.Stop()
	assert.Equal(t, ErrStopped, listenForSyncDiffChunks())
}

func TestListenForSyncInits(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	stop := make(chan struct{})
	listenForSyncInits := func() error {
		return listenForSyncInits(l)(stop)
	}

	var viewID uint64 = 1
	var evt EventIncomingSyncInit

	// Test empty
	evt = EventIncomingSyncInit{
		ViewID:   viewID,
		Response: make(chan SyncInitMetadata, 1),
	}

	l.SyncInitIn <- evt
	assert.NoError(t, listenForSyncInits())
	assert.Equal(t, SyncInitMetadata{PeerID: nil, ViewID: 1, ChunkHashes: nil}, <-evt.Response)

	// Test diff

	// Create the tree and commit it into ledger accounts
	tree := avl.New(store.NewInmem())
	for i := uint64(0); i < 50; i++ {
		tree.Insert([]byte("a"), []byte("b"))
		tree.SetViewID(i)
	}
	assert.NoError(t, l.a.commit(tree))
	expectedChunkHashes := [][blake2b.Size256]byte{
		blake2b.Sum256(tree.DumpDiff(viewID)),
	}

	evt = EventIncomingSyncInit{
		ViewID:   viewID,
		Response: make(chan SyncInitMetadata, 1),
	}

	l.SyncInitIn <- evt
	assert.NoError(t, listenForSyncInits())
	assert.Equal(t, SyncInitMetadata{PeerID: nil, ViewID: 1, ChunkHashes: expectedChunkHashes}, <-evt.Response)

	// Test stop
	close(stop)
	assert.Equal(t, ErrStopped, listenForSyncInits())

	stop = make(chan struct{})

	// Test kill.

	l.Stop()
	assert.Equal(t, ErrStopped, listenForSyncInits())
}

func TestListenForOutOfSyncChecks(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	stop := make(chan struct{})
	listenForOutOfSyncChecks := func() error {
		return listenForOutOfSyncChecks(l)(stop)
	}

	evt := EventIncomingOutOfSyncCheck{
		Response: make(chan *Transaction, 1),
	}

	l.OutOfSyncIn <- evt
	assert.NoError(t, listenForOutOfSyncChecks())
	assert.Equal(t, l.v.loadRoot(), <-evt.Response)

	// Test stop

	close(stop)
	assert.Equal(t, ErrStopped, listenForOutOfSyncChecks())

	stop = make(chan struct{})

	// Test kill.

	l.Stop()
	assert.Equal(t, ErrStopped, listenForOutOfSyncChecks())
}

func TestListenForMissingTXs(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	stop := make(chan struct{})
	listenForMissingTXs := func() error {
		return listenForMissingTXs(l)(stop)
	}

	tx, err := NewTransaction(l.keys, sys.TagTransfer, []byte("lorem ipsum"))
	assert.NoError(t, err)
	tx.rehash()

	assert.NoError(t, l.v.addTransaction(&tx))

	evt := EventIncomingSyncTX{
		IDs:      []common.TransactionID{tx.ID},
		Response: make(chan []Transaction, 1),
	}

	l.SyncTxIn <- evt
	assert.NoError(t, listenForMissingTXs())
	assert.Equal(t, []Transaction{tx}, <-evt.Response)

	// Test stop

	close(stop)
	assert.Equal(t, ErrStopped, listenForMissingTXs())

	stop = make(chan struct{})

	// Test kill.
	l.Stop()
	assert.Equal(t, ErrStopped, listenForMissingTXs())
}

func TestCheckIfOutOfSync(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())
	preferred, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)
	preferred.rehash()

	l.sr.Prefer(preferred)

	stop := make(chan struct{})
	checkIfOutOfSync := func() error {
		return checkIfOutOfSync(l)(stop)
	}

	var wg sync.WaitGroup
	var evt EventOutOfSyncCheck

	// Test empty.

	wg.Add(1)
	go func() {
		assert.NoError(t, call(&wg, checkIfOutOfSync))
	}()
	evt = <-l.OutOfSyncOut
	evt.Result <- []VoteOutOfSync{}
	wg.Wait()

	// Test empty out of sync.

	wg.Add(1)
	go func() {
		assert.NoError(t, call(&wg, checkIfOutOfSync))
	}()
	evt = <-l.OutOfSyncOut
	evt.Result <- []VoteOutOfSync{{}}
	wg.Wait()

	l.sr.decided = true

	// Test out of sync.

	root, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)
	root.rehash()

	wg.Add(1)
	go func() {
		assert.Equal(t, ErrOutOfSync, call(&wg, checkIfOutOfSync))
	}()
	evt = <-l.OutOfSyncOut
	evt.Result <- []VoteOutOfSync{
		{
			Voter: common.AccountID{},
			Root: Transaction{
				ID:     root.ID,
				ViewID: 1,
			},
		},
	}
	wg.Wait()

	// Test not out of sync.

	l.v.saveRoot(&root)

	wg.Add(1)
	go func() {
		assert.NoError(t, call(&wg, checkIfOutOfSync))
	}()
	evt = <-l.OutOfSyncOut
	evt.Result <- []VoteOutOfSync{
		{
			Voter: common.AccountID{},
			Root: Transaction{
				ID:     preferred.ID,
				ViewID: 1,
			},
		},
	}
	wg.Wait()

	// Test error

	wg.Add(1)
	evtError := errors.New("error")
	go func() {
		_ = call(&wg, checkIfOutOfSync)
	}()

	evt = <-l.OutOfSyncOut
	evt.Error <- evtError
	wg.Wait()

	// Test the second select.

	// Test the second select: stop
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrStopped, call(&wg, checkIfOutOfSync))
	}()
	evt = <-l.OutOfSyncOut
	close(stop)
	wg.Wait()
	stop = make(chan struct{})

	// Test the second select: kill.
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrStopped, call(&wg, checkIfOutOfSync))
	}()
	evt = <-l.OutOfSyncOut
	l.Stop()
	wg.Wait()
	l.kill = make(chan struct{})

	// Test the first select.

	// Disable the syncTxOut case.
	l.outOfSyncOut = nil

	// Test the first select: stop.
	close(stop)
	assert.Equal(t, ErrStopped, checkIfOutOfSync())
	stop = make(chan struct{})

	// Test the first select: kill.
	l.Stop()
	assert.Equal(t, ErrStopped, checkIfOutOfSync())
}

func TestSyncMissingTX(t *testing.T) {
	defer leaktest.Check(t)()

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	l := NewLedger(ctx, keys, store.NewInmem())

	stop := make(chan struct{})
	syncMissingTX := func() error {
		return syncMissingTX(l)(stop)
	}

	var wg sync.WaitGroup
	var evt EventSyncTX

	// Test transaction
	p, err := formPayload("400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405", 10)
	if !assert.NoError(t, err) {
		return
	}

	tx, err := NewTransaction(l.keys, sys.TagTransfer, p)
	if !assert.NoError(t, err) {
		return
	}

	tx.rehash()
	tx, err = l.attachSenderToTransaction(tx)
	if !assert.NoError(t, err) {
		return
	}

	snapshot := l.a.snapshot()
	WriteAccountBalance(snapshot, tx.Creator, 1000)
	if !assert.NoError(t, l.a.commit(snapshot)) {
		return
	}

	wg.Add(1)
	go func() {
		assert.NoError(t, call(&wg, syncMissingTX))
	}()
	evt = <-l.SyncTxOut
	evt.Result <- []Transaction{tx}
	wg.Wait()

	_, found := l.v.lookupTransaction(tx.ID)
	assert.True(t, found)

	// Test error
	wg.Add(1)
	go func() {
		err := call(&wg, syncMissingTX)
		assert.EqualError(t, errors.Cause(err), "error")
	}()

	evt = <-l.SyncTxOut
	evt.Error <- errors.New("error")
	wg.Wait()

	// Test the second select.

	// Test the second select: stop
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrStopped, call(&wg, syncMissingTX))
	}()
	evt = <-l.SyncTxOut
	close(stop)
	wg.Wait()
	stop = make(chan struct{})

	// Test the second select: kill.
	wg.Add(1)
	go func() {
		assert.Equal(t, ErrStopped, call(&wg, syncMissingTX))
	}()
	evt = <-l.SyncTxOut
	l.Stop()
	wg.Wait()
	l.kill = make(chan struct{})

	// Test the first select.

	// Disable the syncTxOut case.
	l.syncTxOut = nil

	// Test the first select: stop.
	close(stop)
	assert.Equal(t, ErrStopped, syncMissingTX())
	stop = make(chan struct{})

	// Test the first select: kill.
	l.Stop()
	assert.Equal(t, ErrStopped, syncMissingTX())
}
