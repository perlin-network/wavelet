package wavelet

import (
	"crypto/rand"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/store"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	"sync"
	"testing"
)

type testVote struct {
	v string
}

func (tv *testVote) GetID() string {
	return tv.v
}

func TestCollectVotes(t *testing.T) {
	kv := store.NewInmem()
	accounts := NewAccounts(kv)
	snowballB := 5
	conf.UpdateConfig(conf.WithSnowballBeta(snowballB))
	s := NewSnowball(WithName("test"))

	pubKey := edwards25519.PublicKey{}
	nonce := [blake2b.Size256]byte{}

	t.Run("success - decision made", func(t *testing.T) {
		s.Reset()

		voteC := make(chan vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotes(accounts, s, voteC, wg, conf.GetSnowballK())

		peersNum := conf.GetSnowballK()
		preferred := "a"
		for j := 0; j < snowballB+2; j++ { // +2 because snowball count starts with zero and needs to be greater than B
			for i := 0; i < peersNum; i++ {
				_, _ = rand.Read(pubKey[:])
				voteC <- vote{
					voter: skademlia.NewID("", pubKey, nonce),
					value: &testVote{v: preferred},
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		assert.Equal(t, preferred, s.Preferred().GetID())
	})

	t.Run("success - one of the voters votes wrong, but majority is enough", func(t *testing.T) {
		s.Reset()

		voteC := make(chan vote)
		wg := new(sync.WaitGroup)

		peersNum := 5

		wg.Add(1)
		go CollectVotes(accounts, s, voteC, wg, peersNum)

		for j := 0; j < snowballB+2; j++ {
			for i := 0; i < peersNum; i++ {
				preferred := "a"
				if i == 0 {
					preferred = "b"
				}

				_, _ = rand.Read(pubKey[:])
				voteC <- vote{
					voter: skademlia.NewID("", pubKey, nonce),
					value: &testVote{v: preferred},
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.True(t, s.Decided())
		if !assert.NotNil(t, s.Preferred()) {
			return
		}
		assert.Equal(t, "a", s.Preferred().GetID())
	})

	t.Run("no decision - two voters vote wrong, majority is less than snowballA", func(t *testing.T) {
		s.Reset()

		voteC := make(chan vote)
		wg := new(sync.WaitGroup)

		peersNum := 5
		wg.Add(1)
		go CollectVotes(accounts, s, voteC, wg, peersNum)

		for j := 0; j < snowballB+2; j++ {
			for i := 0; i < peersNum; i++ {
				preferred := "a"
				if i == 0 || i == 1 {
					preferred = "b"
				}

				_, _ = rand.Read(pubKey[:])
				voteC <- vote{
					voter: skademlia.NewID("", pubKey, nonce),
					value: &testVote{v: preferred},
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, s.Decided())
	})

	t.Run("no decision - less than snowballK voters", func(t *testing.T) {
		s.Reset()

		voteC := make(chan vote)
		wg := new(sync.WaitGroup)

		wg.Add(1)
		go CollectVotes(accounts, s, voteC, wg, conf.GetSnowballK())

		peersNum := conf.GetSnowballK() - 1
		preferred := "a"
		for j := 0; j < snowballB+2; j++ {
			for i := 0; i < peersNum; i++ {
				_, _ = rand.Read(pubKey[:])
				voteC <- vote{
					voter: skademlia.NewID("", pubKey, nonce),
					value: &testVote{v: preferred},
				}
			}
		}

		close(voteC)
		wg.Wait()

		assert.False(t, s.Decided())
	})
}
