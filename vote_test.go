package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"sync"
	"testing"
)

func TestTickForFinalization(t *testing.T) {
	t.Parallel()

	snowballK := 10

	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(10))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	// Generate 10 random IDs, each with stake of 1
	keys := make([]*skademlia.ID, 0, snowballK)
	accounts := NewAccounts(store.NewInmem())
	snapshot := accounts.Snapshot()
	for i := 0; i < cap(keys); i++ {
		k, err := skademlia.NewKeys(1, 1)
		assert.NoError(t, err)

		keys = append(keys, k.ID(""))

		WriteAccountStake(snapshot, k.PublicKey(), 1)
	}

	assert.NoError(t, accounts.Commit(snapshot))

	snowball := NewSnowball()

	getTxID := func() TransactionID {
		var id TransactionID

		_, err := rand.Read(id[:])
		assert.NoError(t, err)

		return id
	}

	for i := 0; i < 100; i++ {
		votes := make([]finalizationVote, 0, snowballK)

		var expectedFinalizedBlock *Block

		var expectedFinalizedIdx = rand.Intn(cap(votes))
		// If true, make one ID has the most stake,
		// If false, make one Block has the most transactions.
		var increaseStake = false

		if i%2 == 0 {
			increaseStake = true
			snapshot := accounts.Snapshot()
			WriteAccountStake(snapshot, keys[expectedFinalizedIdx].PublicKey(), sys.MinimumStake*2)
			assert.NoError(t, accounts.Commit(snapshot))
		}

		// Randomly choose a vote that contains the expected preferred block.
		var initialPreferredIdx = rand.Intn(cap(votes))

		// Generate one vote for each ID.
		for i := 0; i < cap(votes); i++ {
			var txIDs []TransactionID
			if i == expectedFinalizedIdx && !increaseStake {
				for n := 0; n < 10; n++ {
					txIDs = append(txIDs, getTxID())
				}
			} else {
				txIDs = append(txIDs, getTxID())
			}

			block := NewBlock(1, accounts.tree.Checksum(), txIDs...)

			if i == expectedFinalizedIdx {
				expectedFinalizedBlock = &block
			}

			if i == initialPreferredIdx {
				snowball.Prefer(newPreferredBlockVote(&block))
			}

			votes = append(votes, finalizationVote{
				voter: keys[i],
				block: &block,
			})
		}

		// Call tick until the snowball requires one more tick to finalized.
		for i := 0; i < conf.GetSnowballBeta(); i++ {
			TickForFinalization(accounts, snowball, votes)
		}

		// At this point the preferred block should be equal to the expected finalized block, but not  decided yet.
		//noinspection GoNilness
		assert.Equal(t, expectedFinalizedBlock.ID, snowball.Preferred().val.(*Block).ID)
		assert.False(t, snowball.Decided())

		// One more tick to finalize
		TickForFinalization(accounts, snowball, votes)

		// At this point, the snowball should be finalized

		assert.True(t, snowball.Decided())
		//noinspection GoNilness
		assert.Equal(t, expectedFinalizedBlock.ID, snowball.Preferred().val.(*Block).ID)

		snowball.Reset()

		// Reset back the stake
		if increaseStake {
			snapshot := accounts.Snapshot()
			WriteAccountStake(snapshot, keys[expectedFinalizedIdx].PublicKey(), 1)
			assert.NoError(t, accounts.Commit(snapshot))
		}
	}

}

func TestCollectVotesForSynce(t *testing.T) {
	t.Parallel()

	snowballK := 10

	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(10))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	// Generate 10 random IDs, each with stake of 1
	keys := make([]*skademlia.ID, 0, snowballK)
	accounts := NewAccounts(store.NewInmem())
	snapshot := accounts.Snapshot()
	for i := 0; i < cap(keys); i++ {
		k, err := skademlia.NewKeys(1, 1)
		assert.NoError(t, err)

		keys = append(keys, k.ID(""))

		WriteAccountStake(snapshot, k.PublicKey(), 1)
	}

	assert.NoError(t, accounts.Commit(snapshot))

	snowball := NewSnowball()

	for i := 0; i < 100; i++ {
		votes := make([]syncVote, 0, snowballK)

		expectedOutOfSync := rand.Intn(2) == 1
		expectedOutOfSyncCount := 0

		// If not equal -1, make one ID has the most stake and equal votes for true and false,
		// If equal -1, make expectedOutOfSync has the majority vote.
		var increaseStake = -1

		if i%2 == 0 {
			increaseStake = rand.Intn(cap(votes))

			snapshot := accounts.Snapshot()
			WriteAccountStake(snapshot, keys[increaseStake].PublicKey(), sys.MinimumStake*2)
			assert.NoError(t, accounts.Commit(snapshot))
		}

		// Generate one vote for each ID.
		for i := 0; i < cap(votes); i++ {
			var outOfSyncVote bool

			if increaseStake != -1 {
				// Make sure each decision has equal votes
				if i == increaseStake {
					outOfSyncVote = expectedOutOfSync
				} else {
					if increaseStake%2 == 0 {
						outOfSyncVote = i%2 == 0
					} else {
						outOfSyncVote = i%2 != 0
					}
				}
			} else {
				if expectedOutOfSyncCount < cap(votes)/2+1 {
					expectedOutOfSyncCount++
					outOfSyncVote = expectedOutOfSync

				} else {
					outOfSyncVote = !expectedOutOfSync
				}
			}

			votes = append(votes, syncVote{
				voter:     keys[i],
				outOfSync: outOfSyncVote,
			})
		}

		var voteChan chan syncVote
		var wg sync.WaitGroup

		// Call tick until the snowball requires one more tick to finalized.
		voteChan = make(chan syncVote)
		go CollectVotesForSync(accounts, snowball, voteChan, &wg, snowballK)
		wg.Add(1)
		for i := 0; i < conf.GetSnowballBeta(); i++ {
			for _, vote := range votes {
				voteChan <- vote
			}
		}
		close(voteChan)
		wg.Wait()

		// At this point the preferred should be equal to the expected, but not  decided yet.
		assert.Equal(t, expectedOutOfSync, snowball.Preferred().val.(*outOfSyncVote).outOfSync)
		assert.False(t, snowball.Decided())

		// One more tick to finalize
		voteChan = make(chan syncVote)
		go CollectVotesForSync(accounts, snowball, voteChan, &wg, snowballK)
		wg.Add(1)
		for _, vote := range votes {
			voteChan <- vote
		}
		close(voteChan)
		wg.Wait()

		// At this point, the snowball should be finalized

		assert.True(t, snowball.Decided())
		assert.Equal(t, expectedOutOfSync, snowball.Preferred().val.(*outOfSyncVote).outOfSync)

		snowball.Reset()

		// Reset back the stake
		if increaseStake != -1 {
			snapshot := accounts.Snapshot()
			WriteAccountStake(snapshot, keys[increaseStake].PublicKey(), 1)
			assert.NoError(t, accounts.Commit(snapshot))
		}
	}
}

//func TestCollectVotesForSync(t *testing.T) {
//	kv := store.NewInmem()
//	accounts := NewAccounts(kv)
//	snowballB := 5
//	conf.Update(conf.WithSnowballBeta(snowballB))
//	s := NewSnowball(WithName("test"))
//
//	pubKey := edwards25519.PublicKey{}
//	nonce := [blake2b.Size256]byte{}
//
//	t.Run("success - decision made", func(t *testing.T) {
//		s.Reset()
//
//		voteC := make(chan syncVote)
//		wg := new(sync.WaitGroup)
//
//		wg.Add(1)
//		go CollectVotesForSync(accounts, s, voteC, wg, conf.GetSnowballK())
//
//		peersNum := conf.GetSnowballK()
//		for j := 0; j < snowballB+2; j++ { // +2 because snowball count starts with zero and needs to be greater than B
//			for i := 0; i < peersNum; i++ {
//				_, _ = rand.Read(pubKey[:])
//				voteC <- syncVote{
//					voter:     skademlia.NewID("", pubKey, nonce),
//					outOfSync: true,
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.True(t, s.Decided())
//		assert.Equal(t, "true", s.Preferred().GetID())
//	})
//
//	t.Run("success - one of the voters votes wrong, but majority is enough", func(t *testing.T) {
//		s.Reset()
//
//		voteC := make(chan syncVote)
//		wg := new(sync.WaitGroup)
//
//		peersNum := 5
//
//		wg.Add(1)
//		go CollectVotesForSync(accounts, s, voteC, wg, peersNum)
//
//		for j := 0; j < snowballB+2; j++ {
//			for i := 0; i < peersNum; i++ {
//				outOfSync := true
//				if i == 0 {
//					outOfSync = false
//				}
//
//				_, _ = rand.Read(pubKey[:])
//				voteC <- syncVote{
//					voter:     skademlia.NewID("", pubKey, nonce),
//					outOfSync: outOfSync,
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.True(t, s.Decided())
//		if !assert.NotNil(t, s.Preferred()) {
//			return
//		}
//		assert.Equal(t, "true", s.Preferred().GetID())
//	})
//
//	t.Run("no decision - two voters vote wrong, majority is less than snowballA", func(t *testing.T) {
//		s.Reset()
//
//		voteC := make(chan syncVote)
//		wg := new(sync.WaitGroup)
//
//		peersNum := 5
//		wg.Add(1)
//		go CollectVotesForSync(accounts, s, voteC, wg, peersNum)
//
//		for j := 0; j < snowballB+2; j++ {
//			for i := 0; i < peersNum; i++ {
//				outOfSync := true
//				if i == 0 || i == 1 {
//					outOfSync = false
//				}
//
//				_, _ = rand.Read(pubKey[:])
//				voteC <- syncVote{
//					voter:     skademlia.NewID("", pubKey, nonce),
//					outOfSync: outOfSync,
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.False(t, s.Decided())
//	})
//
//	t.Run("no decision - less than snowballK voters", func(t *testing.T) {
//		s.Reset()
//
//		voteC := make(chan syncVote)
//		wg := new(sync.WaitGroup)
//
//		wg.Add(1)
//		go CollectVotesForSync(accounts, s, voteC, wg, conf.GetSnowballK())
//
//		peersNum := conf.GetSnowballK() - 1
//		for j := 0; j < snowballB+2; j++ {
//			for i := 0; i < peersNum; i++ {
//				_, _ = rand.Read(pubKey[:])
//				voteC <- syncVote{
//					voter:     skademlia.NewID("", pubKey, nonce),
//					outOfSync: true,
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.False(t, s.Decided())
//	})
//}
//
//func TestCollectVotesForFinalization(t *testing.T) {
//	kv := store.NewInmem()
//	accounts := NewAccounts(kv)
//	snowballB := 5
//
//	defaultSnowballB := conf.GetSnowballBeta()
//	defer conf.Update(conf.WithSnowballBeta(defaultSnowballB))
//
//	conf.Update(conf.WithSnowballBeta(snowballB))
//	s := NewSnowball(WithName("test"))
//
//	pubKey := edwards25519.PublicKey{}
//	nonce := [blake2b.Size256]byte{}
//
//	t.Run("no decision in case of all equal values", func(t *testing.T) {
//		s.Reset()
//
//		voteC := make(chan finalizationVote)
//		wg := new(sync.WaitGroup)
//
//		wg.Add(1)
//		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())
//
//		voters := make([]*skademlia.ID, conf.GetSnowballK())
//		for i := 0; i < len(voters); i++ {
//			_, err := rand.Read(pubKey[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			id := skademlia.NewID("", pubKey, nonce)
//			voters[i] = id
//		}
//
//		roundIDs := make([]RoundID, conf.GetSnowballK()) // each voter votes for different round
//		var roundID RoundID
//		for i := 0; i < len(roundIDs); i++ {
//			_, err := rand.Read(roundID[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			roundIDs[i] = roundID
//		}
//
//		for i := 0; i < snowballB+2; i++ {
//			for j := 0; j < conf.GetSnowballK(); j++ {
//				voteC <- finalizationVote{
//					voter: voters[j],
//					round: &Round{
//						ID: roundIDs[j],
//					},
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.False(t, s.Decided())
//	})
//
//	t.Run("stake majority round wins", func(t *testing.T) {
//		s.Reset()
//
//		var defaultStake uint64 = 10
//		biggestVoter := 0
//
//		snapshot := accounts.Snapshot()
//		voters := make([]*skademlia.ID, conf.GetSnowballK())
//		roundIDs := make([]RoundID, len(voters)) // each voter votes for different round
//
//		var roundID RoundID
//		for i := 0; i < len(voters); i++ {
//			_, err := rand.Read(pubKey[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			id := skademlia.NewID("", pubKey, nonce)
//			voters[i] = id
//
//			stake := defaultStake
//			if i == biggestVoter {
//				stake *= 100
//			}
//
//			WriteAccountStake(snapshot, id.PublicKey(), stake)
//
//			_, err = rand.Read(roundID[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			roundIDs[i] = roundID
//		}
//
//		if !assert.NoError(t, accounts.Commit(snapshot)) {
//			return
//		}
//
//		voteC := make(chan finalizationVote)
//		wg := new(sync.WaitGroup)
//
//		wg.Add(1)
//		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())
//
//		for i := 0; i < snowballB+2; i++ {
//			for j := 0; j < len(voters); j++ {
//				voteC <- finalizationVote{
//					voter: voters[j],
//					round: &Round{
//						ID: roundIDs[j],
//					},
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.True(t, s.Decided())
//		if !assert.NotNil(t, s.Preferred()) {
//			return
//		}
//
//		assert.Equal(t, fmt.Sprintf("%x", roundIDs[biggestVoter]), s.Preferred().GetID())
//	})
//
//	t.Run("transactions num majority round wins", func(t *testing.T) {
//		s.Reset()
//
//		var transactionsNum uint32 = 10
//		biggestVoter := 1
//
//		voters := make([]*skademlia.ID, conf.GetSnowballK())
//		rounds := make([]*Round, len(voters)) // each voter votes for different round
//
//		var roundID RoundID
//		for i := 0; i < len(voters); i++ {
//			_, err := rand.Read(pubKey[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			id := skademlia.NewID("", pubKey, nonce)
//			voters[i] = id
//
//			_, err = rand.Read(roundID[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			transactions := transactionsNum
//			if i == biggestVoter {
//				transactions *= 1000
//			}
//
//			rounds[i] = &Round{
//				ID:           roundID,
//				Transactions: transactions,
//			}
//		}
//
//		voteC := make(chan finalizationVote)
//		wg := new(sync.WaitGroup)
//
//		wg.Add(1)
//		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())
//
//		for i := 0; i < snowballB+2; i++ {
//			for j := 0; j < conf.GetSnowballK(); j++ {
//				voteC <- finalizationVote{
//					voter: voters[j],
//					round: rounds[j],
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.True(t, s.Decided())
//		if !assert.NotNil(t, s.Preferred()) {
//			return
//		}
//
//		assert.Equal(t, fmt.Sprintf("%x", rounds[biggestVoter].ID), s.Preferred().GetID())
//	})
//
//	t.Run("depth majority round wins", func(t *testing.T) {
//		s.Reset()
//
//		var roundDepth uint64 = 1000
//		biggestVoter := 0
//
//		voters := make([]*skademlia.ID, conf.GetSnowballK())
//		rounds := make([]*Round, len(voters)) // each voter votes for different round
//
//		var roundID RoundID
//		for i := 0; i < len(voters); i++ {
//			_, err := rand.Read(pubKey[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			id := skademlia.NewID("", pubKey, nonce)
//			voters[i] = id
//
//			_, err = rand.Read(roundID[:])
//			if !assert.NoError(t, err) {
//				return
//			}
//
//			depth := roundDepth
//			if i == biggestVoter {
//				depth /= 100
//			}
//
//			rounds[i] = &Round{
//				ID:  roundID,
//				End: Transaction{Depth: depth},
//			}
//		}
//
//		voteC := make(chan finalizationVote)
//		wg := new(sync.WaitGroup)
//
//		wg.Add(1)
//		go CollectVotesForFinalization(accounts, s, voteC, wg, conf.GetSnowballK())
//
//		for i := 0; i < snowballB+2; i++ {
//			for j := 0; j < conf.GetSnowballK(); j++ {
//				voteC <- finalizationVote{
//					voter: voters[j],
//					round: rounds[j],
//				}
//			}
//		}
//
//		close(voteC)
//		wg.Wait()
//
//		assert.True(t, s.Decided())
//		if !assert.NotNil(t, s.Preferred()) {
//			return
//		}
//
//		assert.Equal(t, fmt.Sprintf("%x", rounds[biggestVoter].ID), s.Preferred().GetID())
//	})
//}
