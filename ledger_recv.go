package wavelet

import (
	"context"
	"github.com/pkg/errors"
)

func recv(ledger *Ledger) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-ledger.gossipIn:
			return func() error {
				ledger.mu.Lock()
				defer ledger.mu.Unlock()

				err := ledger.addTransaction(evt.TX)

				if err != nil {
					evt.Vote <- err
					return nil
				}

				evt.Vote <- nil

				return nil
			}()
		case evt := <-ledger.queryIn:
			return func() error {
				ledger.mu.Lock()
				defer ledger.mu.Unlock()

				r := evt.Round

				if r.Index < ledger.round { // Respond with the round we decided beforehand.
					round, available := ledger.rounds[r.Index]

					if !available {
						evt.Error <- errors.Errorf("got requested with round %d, but do not have it available", r.Index)
						return nil
					}

					evt.Response <- &round
					return nil
				}

				if err := ledger.addTransaction(r.Root); err != nil { // Add the root in the round to our graph.
					evt.Error <- err
					return nil
				}

				evt.Response <- ledger.snowball.Preferred() // Send back our preferred round info, if we have any.

				return nil
			}()
		case evt := <-ledger.outOfSyncIn:
			return func() error {
				ledger.mu.RLock()
				defer ledger.mu.RUnlock()

				if round, exists := ledger.rounds[ledger.round-1]; exists {
					evt.Response <- &round
				} else {
					evt.Response <- nil
				}

				return nil
			}()
		}
	}
}
