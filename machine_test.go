package wavelet

import (
	"crypto/rand"
	"github.com/perlin-network/noise/identity/ed25519"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

func signalWhenComplete(wg *sync.WaitGroup, l *Ledger, fn transition) {
	fn(l)
	wg.Done()
}

func call(wg *sync.WaitGroup, stop chan struct{}, fn func(stop <-chan struct{}) error) error {
	defer wg.Done()
	return fn(stop)
}

func TestKill(t *testing.T) {
	var wg sync.WaitGroup

	// Test if we can gracefully stop the ledger while it is gossiping.
	l := NewLedger(ed25519.RandomKeys(), store.NewInmem())

	wg.Add(1)
	go signalWhenComplete(&wg, l, gossiping)
	close(l.kill)
	wg.Wait()

	// Test if we can gracefully stop the ledger while it is querying.
	l = NewLedger(ed25519.RandomKeys(), store.NewInmem())
	l.cr.Prefer(Transaction{})

	wg.Add(1)
	go signalWhenComplete(&wg, l, querying)
	close(l.kill)
	wg.Wait()
}

func TestGossipOutTransaction(t *testing.T) {
	l := NewLedger(ed25519.RandomKeys(), store.NewInmem())
	defer close(l.kill)

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

	// Assert no errors.
	assert.NotNil(t, <-evt.Result)

	// Assert that the transactions are the same.
	assert.Equal(t, evt.Payload, out.TX.Payload)

	// Assert that the transaction has a sender attached.
	assert.NotZero(t, out.TX.Timestamp)
	assert.NotEmpty(t, out.TX.ParentIDs)
	assert.NotEmpty(t, out.TX.Sender)
	assert.NotEmpty(t, out.TX.SenderSignature)
}

func TestTransitionFromGossipingToQuerying(t *testing.T) {
	l := NewLedger(ed25519.RandomKeys(), store.NewInmem())
	defer close(l.kill)

	preferred, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)

	preferred.rehash()

	l.cr.Prefer(preferred)

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

	// Run a single iteration of gossiping with a preferred transaction.
	next := make(chan error)
	go func() { next <- gossip(l)(nil) }()
	defer close(next)

	// Collect the gossip that the ledger wanted to send out.
	out := <-l.GossipOut

	// Signal that the gossip was sent out successfully.
	out.Result <- []VoteGossip{{Ok: true}}

	// Assert no errors.
	assert.NotNil(t, <-evt.Result)

	// Assert that we received a signal to transition to querying.
	assert.Equal(t, ErrPreferredSelected, <-next)
}

func TestEnsureGossipReturnsNetworkErrors(t *testing.T) {
	l := NewLedger(ed25519.RandomKeys(), store.NewInmem())
	defer close(l.kill)

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

	// Assert that there were errors.
	assert.NotNil(t, <-evt.Error)

	// Assert that we received no signal.
	assert.Equal(t, nil, <-next)
}

// This test may take a few seconds because of the timeout test
func TestQuery(t *testing.T) {
	l := NewLedger(ed25519.RandomKeys(), store.NewInmem())

	preferred, err := NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)

	preferred.rehash()

	l.cr.Prefer(preferred)
	l.cr.decided = true

	stop := make(chan struct{})
	state := new(stateQuerying)
	query := query(l, state)

	var wg sync.WaitGroup

	// test query

	wg.Add(2)
	go func() {
		assert.Equal(t, ErrConsensusRoundFinished, call(&wg, stop, query))

		// test preferred nil
		assert.Equal(t, ErrConsensusRoundFinished, call(&wg, stop, query))
	}()
	evt := <-l.QueryOut
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

	// re-set the preferred

	preferred, err = NewTransaction(l.keys, sys.TagNop, nil)
	assert.NoError(t, err)
	preferred.rehash()
	l.cr.Prefer(preferred)

	// test query error

	wg.Add(1)
	evtError := errors.New("query error")
	go func() {
		err := call(&wg, stop, query)
		assert.Equal(t, evtError, errors.Cause(err))
	}()

	evt = <-l.QueryOut
	evt.Error <- evtError
	wg.Wait()

	// test timeout

	err = query(stop)
	assert.Equal(t, ErrTimeout, errors.Cause(err))

	// test stop

	close(stop)
	assert.Equal(t, ErrStopped, query(stop))

	stop = make(chan struct{})

	// test kill

	stop = make(chan struct{})
	close(l.kill)
	assert.Equal(t, ErrStopped, query(stop))
}

func TestListenForQueries(t *testing.T) {
	l := NewLedger(ed25519.RandomKeys(), store.NewInmem())

	var rootId common.TransactionID
	var rootViewId uint64 = 100
	_, err := rand.Read(rootId[:])
	assert.NoError(t, err)
	root := &Transaction{
		ID:     rootId,
		ViewID: rootViewId,
	}

	l.v.saveRoot(root)

	var expectedErr unsafe.Pointer

	fn := listenForQueries(l)

	stop := make(chan struct{})

	var done sync.WaitGroup
	done.Add(1)

	proceed := make(chan struct{})

	go func() {
		defer done.Done()
		var err error

		for {
			_, ok := <-proceed
			if !ok {
				return
			}

			err = fn(stop)

			if val := atomic.LoadPointer(&expectedErr); val != nil {
				expected := *(*error)(val)

				assert.Equal(t, expected, errors.Cause(err))
			}
		}
	}()

	proceed <- struct{}{}

	// test root response

	evt := EventIncomingQuery{
		TX: Transaction{
			ViewID: rootViewId,
		},
		Response: make(chan *Transaction, 1),
		Error:    make(chan error, 1),
	}
	l.QueryIn <- evt
	tx := <-evt.Response
	assert.Equal(t, l.v.loadRoot().ID, tx.ID)

	proceed <- struct{}{}

	// test nil response

	evt = EventIncomingQuery{
		TX:       Transaction{},
		Response: make(chan *Transaction, 1),
		Error:    make(chan error, 1),
	}
	l.QueryIn <- evt
	tx = <-evt.Response
	assert.Nil(t, tx)

	proceed <- struct{}{}

	// test preferred response

	var preferredId common.TransactionID
	_, err = rand.Read(preferredId[:])
	assert.NoError(t, err)
	preferred := Transaction{ID: preferredId}
	l.cr.Prefer(preferred)

	evt = EventIncomingQuery{
		Response: make(chan *Transaction, 1),
		Error:    make(chan error, 1),
	}

	l.QueryIn <- evt
	tx = <-evt.Response
	assert.Equal(t, preferred.ID, tx.ID)

	proceed <- struct{}{}

	// test kill
	atomic.StorePointer(&expectedErr, unsafe.Pointer(&ErrStopped))
	l.kill <- struct{}{}

	proceed <- struct{}{}

	// test stop
	atomic.StorePointer(&expectedErr, unsafe.Pointer(&ErrStopped))
	stop <- struct{}{}

	close(proceed)

	done.Wait()
}
