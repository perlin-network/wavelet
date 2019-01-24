package node

import (
	"encoding/binary"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/peer"
	"github.com/perlin-network/wavelet"
)

const (
	minimumStake = 100
)

var _ sybil = (*stake)(nil)

type stake struct {
	query
}

func (s stake) weigh(peers []peer.ID, responses []bool, tx *wire.Transaction) (positives float32) {
	var stakes []uint64

	maxStake := uint64(0)

	// Get stakes of all K peers (with minimum stake into consideration), and find the max stake.
	for _, peer := range peers {
		stake := uint64(minimumStake)

		var account *wavelet.Account
		var err error

		s.LedgerDo(func(l wavelet.LedgerInterface) {
			account = wavelet.NewAccount(l.(*wavelet.Ledger), peer.PublicKey)
		})

		if err == nil {
			if val, exists := account.Load("stake"); exists {
				if s := binary.LittleEndian.Uint64(val); s > stake {
					stake = s
				}
			}
		}

		if stake > maxStake {
			maxStake = stake
		}

		stakes = append(stakes, stake)
	}

	// Calculate votes based off of stake.
	for i, stake := range stakes {
		if responses[i] {
			vote := float32(stake/maxStake) / float32(len(peers))
			positives += vote
		}
	}

	return positives
}
