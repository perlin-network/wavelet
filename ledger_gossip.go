package wavelet

import (
	"context"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"time"
)

func gossip(ledger *Ledger) func(ctx context.Context) error {
	broadcastNops := false

	return func(ctx context.Context) error {
		ledger.syncingCond.L.Lock()
		for ledger.syncing {
			ledger.syncingCond.Wait()
		}
		ledger.syncingCond.L.Unlock()

		snapshot := ledger.accounts.snapshot()

		var tx Transaction
		var err error

		var Result chan<- Transaction
		var Error chan<- error

		select {
		case <-ctx.Done():
			return nil
		case item := <-ledger.broadcastQueue:
			tx = Transaction{
				Tag:              item.Tag,
				Payload:          item.Payload,
				Creator:          item.Creator,
				CreatorSignature: item.Signature,
			}

			Result = item.Result
			Error = item.Error
		case <-time.After(1 * time.Millisecond):
			if !broadcastNops {
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}

				return nil
			}

			// Check if we have enough money available to create and broadcast a nop transaction.

			if balance, _ := ReadAccountBalance(snapshot, ledger.keys.PublicKey()); balance < sys.TransactionFeeAmount {
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}

				return nil
			}

			tx = NewTransaction(ledger.keys, sys.TagNop, nil)
		}

		tx, err = ledger.attachSenderToTransaction(tx)

		if err != nil {
			if Error != nil {
				Error <- errors.Wrap(err, "failed to sign off transaction")
			}
			return nil
		}

		evt := EventGossip{
			TX:     tx,
			Result: make(chan []VoteGossip, 1),
			Error:  make(chan error, 1),
		}

		select {
		case <-ctx.Done():
			if Error != nil {
				Error <- ErrStopped
			}

			return nil
		case <-time.After(1 * time.Second):
			if Error != nil {
				Error <- errors.New("gossip queue is full")
			}

			return nil
		case ledger.gossipOut <- evt:
		}

		var votes []VoteGossip

		select {
		case <-ctx.Done():
			if Error != nil {
				Error <- ErrStopped
			}

			return nil
		case err := <-evt.Error:
			if err != nil {
				if Error != nil {
					Error <- errors.Wrap(err, "got an error gossiping transaction out")
				}
				return nil
			}
		case <-time.After(1 * time.Second):
			if Error != nil {
				Error <- errors.New("did not get back a gossip response")
			}
		case votes = <-evt.Result:
		}

		if len(votes) == 0 {
			return nil
		}

		successful := false

		for _, vote := range votes {
			if vote.Ok {
				successful = true
				break
			}
		}

		if !successful {
			if Error != nil {
				Error <- errors.Errorf("none of our peers find %x valid", evt.TX.ID)
			}

			return nil
		}

		// Double-check that after gossiping, we have not progressed a single view ID and
		// that the transaction is still valid for us to add to our view-graph.

		if err := ledger.addTransaction(tx); err != nil {
			if Error != nil {
				Error <- err
			}

			return nil
		}

		/** At this point, the transaction was successfully added to our view-graph. **/

		// If we have nothing else to broadcast and we are not broadcasting out
		// nop transactions, then start broadcasting out nop transactions.
		if len(ledger.broadcastQueue) == 0 && !broadcastNops && ledger.snowball.Preferred() == nil {
			broadcastNops = true
		}

		if ledger.snowball.Preferred() != nil {
			broadcastNops = false
		}

		if Result != nil {
			Result <- tx
		}

		return nil
	}
}
