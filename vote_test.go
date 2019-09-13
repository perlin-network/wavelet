package wavelet

import (
	"crypto/rand"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	"sync"
	"testing"
)

func TestCollectVotesForSync(t *testing.T) {
	kv := store.NewInmem()
	accounts := NewAccounts(kv)
	snowballB := 5
	conf.Update(conf.WithSnowballBeta(snowballB))
	s := NewSnowball(WithName("test"))

	pubKey := edwards25519.PublicKey{}
	nonce := [blake2b.Size256]byte{}

	t.Run("success - decision made", func(t *testing.T) {
		s.Reset()

		voteC := make(chan syncVote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, s, voteC, wg, conf.GetSnowballK())

		peersNum := conf.GetSnowballK()
		for j := 0; j < snowballB+2; j++ { // +2 because snowball count starts with zero and needs to be greater than B
			for i := 0; i < peersNum; i++ {
				_, _ = rand.Read(pubKey[:])
				voteC <- syncVote{
					voter:     skademlia.NewID("", pubKey, nonce),
					outOfSync: true,
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		assert.Equal(t, "true", s.Preferred().GetID())
	})

	t.Run("success - one of the voters votes wrong, but majority is enough", func(t *testing.T) {
		s.Reset()

		voteC := make(chan syncVote)
		wg := new(sync.WaitGroup)

		peersNum := 5

		wg.Add(1)
		go CollectVotesForSync(accounts, s, voteC, wg, peersNum)

		for j := 0; j < snowballB+2; j++ {
			for i := 0; i < peersNum; i++ {
				outOfSync := true
				if i == 0 {
					outOfSync = false
				}

				_, _ = rand.Read(pubKey[:])
				voteC <- syncVote{
					voter:     skademlia.NewID("", pubKey, nonce),
					outOfSync: outOfSync,
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		if !assert.NotNil(t, s.Preferred()) {
			return
		}
		assert.Equal(t, "true", s.Preferred().GetID())
	})

	t.Run("no decision - two voters vote wrong, majority is less than snowballA", func(t *testing.T) {
		s.Reset()

		voteC := make(chan syncVote)
		wg := new(sync.WaitGroup)

		peersNum := 5
		wg.Add(1)
		go CollectVotesForSync(accounts, s, voteC, wg, peersNum)

		for j := 0; j < snowballB+2; j++ {
			for i := 0; i < peersNum; i++ {
				outOfSync := true
				if i == 0 || i == 1 {
					outOfSync = false
				}

				_, _ = rand.Read(pubKey[:])
				voteC <- syncVote{
					voter:     skademlia.NewID("", pubKey, nonce),
					outOfSync: outOfSync,
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, s.Decided())
	})

	t.Run("no decision - less than snowballK voters", func(t *testing.T) {
		s.Reset()

		voteC := make(chan syncVote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, s, voteC, wg, conf.GetSnowballK())

		peersNum := conf.GetSnowballK() - 1
		for j := 0; j < snowballB+2; j++ {
			for i := 0; i < peersNum; i++ {
				_, _ = rand.Read(pubKey[:])
				voteC <- syncVote{
					voter:     skademlia.NewID("", pubKey, nonce),
					outOfSync: true,
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, s.Decided())
	})
}

func TestCollectVotesForFinalization(t *testing.T) {
	kv := store.NewInmem()
	accounts := NewAccounts(kv)
	snowballB := 5

	defaultSnowballB := conf.GetSnowballBeta()
	defer conf.Update(conf.WithSnowballBeta(defaultSnowballB))

	conf.Update(conf.WithSnowballBeta(snowballB))
	s := NewSnowball(WithName("test"))

	pubKey := edwards25519.PublicKey{}
	nonce := [blake2b.Size256]byte{}

	t.Run("no decision in case of all equal values", func(t *testing.T) {
		s.Reset()

		voteC := make(chan finalizationVote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())

		voters := make([]*skademlia.ID, conf.GetSnowballK())
		for i := 0; i < len(voters); i++ {
			_, err := rand.Read(pubKey[:])
			if !assert.NoError(t, err) {
				return
			}

			id := skademlia.NewID("", pubKey, nonce)
			voters[i] = id
		}

		roundIDs := make([]RoundID, conf.GetSnowballK()) // each voter votes for different round
		var roundID RoundID
		for i := 0; i < len(roundIDs); i++ {
			_, err := rand.Read(roundID[:])
			if !assert.NoError(t, err) {
				return
			}

			roundIDs[i] = roundID
		}

		for i := 0; i < snowballB+2; i++ {
			for j := 0; j < conf.GetSnowballK(); j++ {
				voteC <- finalizationVote{
					voter: voters[j],
					round: &Round{
						ID: roundIDs[j],
					},
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, s.Decided())
	})

	t.Run("stake majority round wins", func(t *testing.T) {
		s.Reset()

		var defaultStake uint64 = 10
		biggestVoter := 0

		snapshot := accounts.Snapshot()
		voters := make([]*skademlia.ID, conf.GetSnowballK())
		roundIDs := make([]RoundID, len(voters)) // each voter votes for different round

		var roundID RoundID
		for i := 0; i < len(voters); i++ {
			_, err := rand.Read(pubKey[:])
			if !assert.NoError(t, err) {
				return
			}

			id := skademlia.NewID("", pubKey, nonce)
			voters[i] = id

			stake := defaultStake
			if i == biggestVoter {
				stake *= 100
			}

			WriteAccountStake(snapshot, id.PublicKey(), stake)

			_, err = rand.Read(roundID[:])
			if !assert.NoError(t, err) {
				return
			}

			roundIDs[i] = roundID
		}

		if !assert.NoError(t, accounts.Commit(snapshot)) {
			return
		}

		voteC := make(chan finalizationVote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())

		for i := 0; i < snowballB+2; i++ {
			for j := 0; j < len(voters); j++ {
				voteC <- finalizationVote{
					voter: voters[j],
					round: &Round{
						ID: roundIDs[j],
					},
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		if !assert.NotNil(t, s.Preferred()) {
			return
		}

		assert.Equal(t, fmt.Sprintf("%x", roundIDs[biggestVoter]), s.Preferred().GetID())
	})

	t.Run("transactions num majority round wins", func(t *testing.T) {
		s.Reset()

		var transactionsNum uint32 = 10
		biggestVoter := 1

		voters := make([]*skademlia.ID, conf.GetSnowballK())
		rounds := make([]*Round, len(voters)) // each voter votes for different round

		var roundID RoundID
		for i := 0; i < len(voters); i++ {
			_, err := rand.Read(pubKey[:])
			if !assert.NoError(t, err) {
				return
			}

			id := skademlia.NewID("", pubKey, nonce)
			voters[i] = id

			_, err = rand.Read(roundID[:])
			if !assert.NoError(t, err) {
				return
			}

			transactions := transactionsNum
			if i == biggestVoter {
				transactions *= 1000
			}

			rounds[i] = &Round{
				ID:           roundID,
				Transactions: transactions,
			}
		}

		voteC := make(chan finalizationVote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())

		for i := 0; i < snowballB+2; i++ {
			for j := 0; j < conf.GetSnowballK(); j++ {
				voteC <- finalizationVote{
					voter: voters[j],
					round: rounds[j],
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		if !assert.NotNil(t, s.Preferred()) {
			return
		}

		assert.Equal(t, fmt.Sprintf("%x", rounds[biggestVoter].ID), s.Preferred().GetID())
	})

	t.Run("depth majority round wins", func(t *testing.T) {
		s.Reset()

		var roundDepth uint64 = 1000
		biggestVoter := 0

		voters := make([]*skademlia.ID, conf.GetSnowballK())
		rounds := make([]*Round, len(voters)) // each voter votes for different round

		var roundID RoundID
		for i := 0; i < len(voters); i++ {
			_, err := rand.Read(pubKey[:])
			if !assert.NoError(t, err) {
				return
			}

			id := skademlia.NewID("", pubKey, nonce)
			voters[i] = id

			_, err = rand.Read(roundID[:])
			if !assert.NoError(t, err) {
				return
			}

			depth := roundDepth
			if i == biggestVoter {
				depth /= 100
			}

			rounds[i] = &Round{
				ID:  roundID,
				End: Transaction{Depth: depth},
			}
		}

		voteC := make(chan finalizationVote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())

		for i := 0; i < snowballB+2; i++ {
			for j := 0; j < conf.GetSnowballK(); j++ {
				voteC <- finalizationVote{
					voter: voters[j],
					round: rounds[j],
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		if !assert.NotNil(t, s.Preferred()) {
			return
		}

		assert.Equal(t, fmt.Sprintf("%x", rounds[biggestVoter].ID), s.Preferred().GetID())
	})
}
