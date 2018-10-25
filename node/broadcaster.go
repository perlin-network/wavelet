package node

import (
	"time"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/security"
	"github.com/perlin-network/wavelet/stats"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/system"
	"github.com/perlin-network/graph/wire"
)

var (
	RetryDelay = 10 * time.Millisecond
)

type broadcaster struct {
	*Wavelet
}

// MakeTransaction creates a new transaction, appends a nonce to it, signs it, and links it with descendants that will
// maximize the likelihood the transaction will get accepted.
func (b *broadcaster) MakeTransaction(tag string, payload []byte) *wire.Transaction {
	var parents []string
	var nonce uint64
	var err error

	b.Ledger.Do(func(l *wavelet.Ledger) {
		parents, err = l.FindEligibleParents()
		if err == nil {
			nonce, err = b.Wallet.NextNonce(l)
		}
	})

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to find eligible parents or figure out the next available nonce.")
	}

	wired := &wire.Transaction{
		Sender:  b.Wallet.PublicKeyHex(),
		Nonce:   nonce,
		Parents: parents,
		Tag:     tag,
		Payload: payload,
	}

	encoded, err := wired.Marshal()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to marshal wired transaction.")
	}

	wired.Signature = security.Sign(b.Wallet.PrivateKey, encoded)

	return wired
}

// BroadcastTransaction broadcasts a transaction and greedily attempts to get it accepted by broadcasting
// nops which are built on top of our transaction.
func (b *broadcaster) BroadcastTransaction(wired *wire.Transaction) {
	start := time.Now()

	var err error
	var id string
	var successful bool
	var tx *database.Transaction

	b.Ledger.Do(func(l *wavelet.Ledger) {
		id, successful, err = l.RespondToQuery(wired)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to insert our own broadcasted transaction into the ledger.")
		}

		if successful {
			tx, err = l.GetBySymbol(id)
		}
	})

	if !successful {
		return
	}

	if err != nil {
		log.Warn().Err(err).Msg("Failed to find our validated transaction in our database.")
		return
	}

	// Query our wired transaction.
	for i := 0; ; i++ {
		err = b.Query(wired)

		if err != nil {
			log.Warn().
				Err(err).
				Int("num_attempts", i).
				Msg("Failed to get our transaction validated by K peers.")

			time.Sleep(RetryDelay)

			continue
		}

		break
	}

	b.Ledger.Do(func(l *wavelet.Ledger) {
		err = l.HandleSuccessfulQuery(tx)
	})

	if err != nil {
		log.Warn().Err(err).Msg("Failed to process our transaction which was successfully queried.")
		return
	}

	var nop *wire.Transaction
	var nopID string

	for attempt := 0; ; attempt++ {
		shouldBreak := false
		shouldReturn := false

		b.Ledger.Do(func(l *wavelet.Ledger) {
			if l.WasAccepted(id) {
				shouldBreak = true
				return
			}

			confidence := l.CountAscendants(id, system.Beta2+1)

			if confidence > system.Beta2 {
				log.Error().Msg("Failed to get our transaction accepted in spite of broadcasting > Beta2 nops.")
				shouldReturn = true
				return
			}
		})

		if shouldBreak {
			break
		}

		if shouldReturn {
			return
		}

		nop = b.MakeTransaction(params.NopTag, nil)

		b.Ledger.Do(func(l *wavelet.Ledger) {
			nopID, successful, err = l.RespondToQuery(nop)

			if err != nil {
				log.Fatal().Err(err).Msg("Failed to insert our nop into the ledger.")
			}
		})

		if !successful {
			continue
		}

		for i := 0; ; i++ {
			err = b.Query(nop)

			if err != nil {
				log.Warn().
					Err(err).
					Int("num_attempts", i).
					Msg("Failed to get our nop validated by K peers.")

				time.Sleep(RetryDelay)

				continue
			}

			break
		}

		b.Ledger.Do(func(l *wavelet.Ledger) {
			nopDB, err := l.GetBySymbol(nopID)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to find our nop in our database.")
				shouldReturn = true
				return
			}

			err = l.HandleSuccessfulQuery(nopDB)

			if err != nil {
				log.Warn().Err(err).Msg("Failed to process our nop which was successfully queried.")
				time.Sleep(RetryDelay)
				return
			}
		})

		if shouldReturn {
			return
		}
	}

	stats.SetConsensusDuration(time.Now().Sub(start).Seconds())
	log.Debug().Str("id", id).Str("tag", wired.Tag).Msg("Successfully broadcasted transaction.")
}
