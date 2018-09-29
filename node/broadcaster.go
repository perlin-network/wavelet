package node

import (
	"github.com/perlin-network/graph/system"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"time"
)

var (
	RetryDelay = 100 * time.Millisecond
)

type broadcaster struct {
	*Wavelet
}

func (b *broadcaster) MakeTransaction(tag string, payload []byte) *wire.Transaction {
	parents, err := b.Ledger.FindEligibleParents()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to find eligible parents.")
	}

	nonce, err := b.Wallet.NextNonce(b.Ledger)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to figure out the next available nonce from our wallet.")
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

func (b *broadcaster) BroadcastTransaction(wired *wire.Transaction) {
	id, successful, err := b.Ledger.RespondToQuery(wired)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to insert our own broadcasted transaction into the ledger.")
	}

	if !successful {
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

	tx, err := b.Ledger.GetBySymbol(id)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find our validated transaction in our database.")
		return
	}

	err = b.Ledger.HandleSuccessfulQuery(tx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to process our transaction which was successfully queried.")
		return
	}

	for attempt := 0; ; attempt++ {
		if b.Ledger.WasAccepted(id) {
			break
		}

		confidence := b.Ledger.CountAscendants(id, system.Beta2+1)

		if confidence > system.Beta2 {
			log.Error().Msg("Failed to get our transaction accepted in spite of broadcasting > Beta2 nops.")
			return
		}

		nop := b.MakeTransaction("nop", nil)

		nopID, successful, err := b.Ledger.RespondToQuery(nop)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to insert our nop into the ledger.")
		}

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

		nopDB, err := b.Ledger.GetBySymbol(nopID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to find our nop in our database.")
			return
		}

		err = b.Ledger.HandleSuccessfulQuery(nopDB)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to process our nop which was successfully queried.")
			continue
		}

		time.Sleep(RetryDelay)
	}

	log.Debug().Str("id", id).Interface("tx", wired).Msgf("Received a transaction, and voted '%t' for it.", successful)
}
