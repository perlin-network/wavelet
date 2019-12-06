// +build unit

package wavelet

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"sync"
	"testing"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
)

func TestFinalizationVoteWithoutBlock(t *testing.T) {
	v := &finalizationVote{
		voter: getRandomID(t),
	}

	assert.Equal(t, ZeroVoteID, v.ID())
	assert.Nil(t, v.Value())
	assert.Zero(t, v.Length())
}

func TestCalculateTallies(t *testing.T) {
	getTxID := func(count int) []TransactionID {
		var ids []TransactionID

		var id TransactionID
		for i := 0; i < count; i++ {
			_, err := rand.Read(id[:]) // nolint:gosec
			assert.NoError(t, err)

			ids = append(ids, id)
		}

		return ids
	}

	accounts := NewAccounts(store.NewInmem())
	snapshot := accounts.Snapshot()

	var votes []Vote
	expectedTallies := make(map[VoteID]float64)

	baseStake := sys.MinimumStake * 2

	nodeIDCount := uint16(0)
	var nodeID MerkleNodeID

	// Vote 1: empty vote
	{
		voterID := getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), baseStake)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: nil,
		})

		expectedTallies[ZeroVoteID] = 0.3037300177619893
	}

	// Vote 2: has the highest stake.
	{
		nodeIDCount++
		binary.BigEndian.PutUint16(nodeID[:], nodeIDCount)
		block, _ := NewBlock(1, nodeID, getTxID(2)...)

		voterID := getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), baseStake*10)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: &block,
		})

		expectedTallies[block.ID] = 0.03374777975133215
	}

	// Vote 3: has the highest transactions.
	{
		nodeIDCount++
		binary.BigEndian.PutUint16(nodeID[:], nodeIDCount)
		block, _ := NewBlock(1, nodeID, getTxID(10)...)

		voterID := getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), baseStake*2)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: &block,
		})

		expectedTallies[block.ID] = 0.04795737122557727
	}

	// Vote 4: has two responses, and third highest stake and transactions.
	{
		nodeIDCount++
		binary.BigEndian.PutUint16(nodeID[:], nodeIDCount)
		block, _ := NewBlock(1, nodeID, getTxID(4)...)

		// First response

		voterID := getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), baseStake*4)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: &block,
		})

		copyBlock := block

		// Second response

		voterID = getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), baseStake*4)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: &copyBlock,
		})

		expectedTallies[block.ID] = 0.37300177619893427
	}

	// Vote 5: Second highest stake and transactions.
	{
		nodeIDCount++
		binary.BigEndian.PutUint16(nodeID[:], nodeIDCount)
		block, _ := NewBlock(1, nodeID, getTxID(9)...)

		voterID := getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), baseStake*9)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: &block,
		})

		expectedTallies[block.ID] = 0.24156305506216696
	}

	// Vote 6: has zero stake (thus minimum stake), and 1 transactions.
	// It's expected that only this vote will have 0 tally because it has the lowest stake and transactions (profit).
	{
		nodeIDCount++
		binary.BigEndian.PutUint16(nodeID[:], nodeIDCount)
		block, _ := NewBlock(1, nodeID, getTxID(1)...)

		voterID := getRandomID(t)
		WriteAccountStake(snapshot, voterID.PublicKey(), 0)

		votes = append(votes, &finalizationVote{
			voter: voterID,
			block: &block,
		})

		expectedTallies[block.ID] = 0
	}

	assert.NoError(t, accounts.Commit(snapshot))

	tallies := calculateTallies(accounts, votes)
	assert.Len(t, tallies, 6)

	// Because of floating point inaccuracy, we convert it to an integer.
	toInt64 := func(v float64) int64 {
		// Use 10 ^ 15 because it seems the inaccuracy is happening at 10 ^ -16.
		exp := math.Pow10(15)
		return int64(exp * v)
	}

	for _, vote := range tallies {
		assert.Equal(t, toInt64(expectedTallies[vote.ID()]), toInt64(vote.Tally()))
	}
}

func TestTickForFinalization(t *testing.T) {
	getTxIDs := func(count int) []TransactionID {
		var ids []TransactionID

		for i := 0; i < count; i++ {
			var id TransactionID
			_, err := rand.Read(id[:]) // nolint:gosec
			assert.NoError(t, err)
			ids = append(ids, id)
		}

		return ids
	}

	snowballK := 10
	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(5))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	snowball := NewSnowball()

	t.Run("no decision in case of all equal tallies", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		votes := make([]Vote, 0, snowballK)

		for i := 0; i < cap(votes); i++ {
			block, err := NewBlock(1, accounts.tree.Checksum(), getTxIDs(1)...)
			if !assert.NoError(t, err) {
				return
			}

			votes = append(votes, &finalizationVote{
				voter: getRandomID(t),
				block: &block,
			})
		}

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			snowball.Tick(calculateTallies(accounts, votes))
		}

		assert.False(t, snowball.Decided())
		assert.Nil(t, snowball.Preferred())
	})

	t.Run("no decision in case of 20% empty votes", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		snapshot := accounts.Snapshot()
		votes := make([]Vote, 0, snowballK)

		_block, err := NewBlock(1, accounts.tree.Checksum(), getTxIDs(1)...)
		if !assert.NoError(t, err) {
			return
		}

		for i := 0; i < cap(votes); i++ {
			voter := getRandomID(t)

			var block *Block
			if i < int(conf.GetSnowballAlpha()*float64(cap(votes))) {
				block = &_block

				// Make sure that the amount of stakes does not matter in case of 20% empty votes.
				WriteAccountStake(snapshot, voter.PublicKey(), sys.MinimumStake*10)
			}

			votes = append(votes, &finalizationVote{
				voter: voter,
				block: block,
			})
		}

		assert.NoError(t, accounts.Commit(snapshot))

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			snowball.Tick(calculateTallies(accounts, votes))
		}

		assert.False(t, snowball.Decided())
		assert.Equal(t, _block, *snowball.Preferred().Value().(*Block))
	})

	t.Run("stake majority block wins", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		biggestStakeIdx := 0
		snapshot := accounts.Snapshot()

		votes := make([]Vote, 0, snowballK)

		for i := 0; i < cap(votes); i++ {
			block, err := NewBlock(1, accounts.tree.Checksum(), getTxIDs(1)...)
			if !assert.NoError(t, err) {
				return
			}

			voter := getRandomID(t)

			stake := sys.MinimumStake
			if i == biggestStakeIdx {
				stake++
			}

			WriteAccountStake(snapshot, voter.PublicKey(), stake)

			votes = append(votes, &finalizationVote{
				voter: voter,
				block: &block,
			})
		}

		assert.NoError(t, accounts.Commit(snapshot))

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			snowball.Tick(calculateTallies(accounts, votes))
		}

		assert.True(t, snowball.Decided())
		assert.Equal(t, votes[biggestStakeIdx].(*finalizationVote).block, snowball.Preferred().Value())
	})

	t.Run("transactions num majority block wins", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())

		snowball.Reset()

		biggestTxNumIdx := 0
		votes := make([]Vote, 0, snowballK)

		for i := 0; i < cap(votes); i++ {
			num := 2
			if i == biggestTxNumIdx {
				num++
			}

			block, err := NewBlock(1, accounts.tree.Checksum(), getTxIDs(num)...)
			if !assert.NoError(t, err) {
				return
			}

			votes = append(votes, &finalizationVote{
				voter: getRandomID(t),
				block: &block,
			})
		}

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			snowball.Tick(calculateTallies(accounts, votes))
		}

		assert.True(t, snowball.Decided())
		assert.Equal(t, votes[biggestTxNumIdx].(*finalizationVote).block, snowball.Preferred().Value())
	})
}

func TestCollectVotesForSync(t *testing.T) {
	snowballK := 10
	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(5))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	snowball := NewSnowball()

	t.Run("success - decision made", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		votes := make([]Vote, 0, snowballK)
		voteC := make(chan Vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

		for i := 0; i < cap(votes); i++ {
			votes = append(votes, &syncVote{
				voter:     getRandomID(t),
				outOfSync: true,
			})
		}

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			for n := range votes {
				voteC <- votes[n]
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, snowball.Decided())
		assert.True(t, *snowball.preferred.Value().(*bool))
	})

	t.Run("success - one of the voters votes wrong, but majority is enough", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		votes := make([]Vote, 0, snowballK)
		voteC := make(chan Vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

		for i := 0; i < cap(votes); i++ {
			outOfSync := true
			if i == 0 {
				outOfSync = false
			}

			votes = append(votes, &syncVote{
				voter:     getRandomID(t),
				outOfSync: outOfSync,
			})
		}

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			for n := range votes {
				voteC <- votes[n]
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, snowball.Decided())
		assert.True(t, *snowball.preferred.Value().(*bool))
	})

	t.Run("success - 50-50 votes, but one vote has the highest stake", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snapshot := accounts.Snapshot()
		snowball.Reset()

		votes := make([]Vote, 0, snowballK)
		voteC := make(chan Vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

		for i := 0; i < cap(votes); i++ {
			voter := getRandomID(t)

			outOfSync := false
			if i%2 == 0 {
				outOfSync = true
			}

			stake := sys.MinimumStake
			if i == 0 {
				stake *= 10
			}
			WriteAccountStake(snapshot, voter.PublicKey(), stake)

			votes = append(votes, &syncVote{
				voter:     voter,
				outOfSync: outOfSync,
			})
		}

		assert.NoError(t, accounts.Commit(snapshot))

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			for n := range votes {
				voteC <- votes[n]
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, snowball.Decided())
		assert.True(t, *snowball.Preferred().Value().(*bool))
	})

	t.Run("no decision - 50-50 votes", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		votes := make([]Vote, 0, snowballK)
		voteC := make(chan Vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

		for i := 0; i < cap(votes); i++ {
			voter := getRandomID(t)

			outOfSync := true
			if i%2 == 0 {
				outOfSync = false
			}

			votes = append(votes, &syncVote{
				voter:     voter,
				outOfSync: outOfSync,
			})
		}

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			for n := range votes {
				voteC <- votes[n]
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, snowball.Decided())
	})

	t.Run("no decision - less than snowballK voter", func(t *testing.T) {
		accounts := NewAccounts(store.NewInmem())
		snowball.Reset()

		votes := make([]Vote, 0, snowballK)
		voteC := make(chan Vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

		for i := 0; i < cap(votes)-1; i++ {
			votes = append(votes, &syncVote{
				voter:     getRandomID(t),
				outOfSync: true,
			})
		}

		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
			for n := range votes {
				voteC <- votes[n]
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, snowball.Decided())
	})
}
