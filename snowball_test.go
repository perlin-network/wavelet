// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package wavelet

import (
	"crypto/rand"
	"encoding/binary"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	mrand "math/rand"
	"testing"
)

type testVote struct {
	voteID VoteID
	voter  *skademlia.ID
	tally  float64
	value  interface{}
}

func newTestVote(id int, tally float64, voter *skademlia.ID, value interface{}) *testVote {
	var voteID VoteID
	// To avoid conflict with ZeroVoteID, make sure the id is never zero.
	binary.BigEndian.PutUint16(voteID[:], uint16(id+1))

	return &testVote{
		voteID: voteID,
		tally:  tally,
		voter:  voter,
		value:  value,
	}
}

func (t *testVote) ID() VoteID {
	return t.voteID
}

func (t *testVote) VoterID() AccountID {
	return t.voter.PublicKey()
}

func (t *testVote) Length() float64 {
	// Not applicable on this test
	return 0
}

func (t *testVote) Value() interface{} {
	return t.value
}

func (t *testVote) Tally() float64 {
	return t.tally
}

func (t *testVote) SetTally(v float64) {
	t.tally = v
}

func getRandomID(t *testing.T) *skademlia.ID {
	pubKey := edwards25519.PublicKey{}
	_, err := rand.Read(pubKey[:])
	assert.NoError(t, err)
	return skademlia.NewID("", pubKey, [blake2b.Size256]byte{})
}

func TestSnowball(t *testing.T) {
	t.Parallel()
	snowballBeta := 10
	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(snowballBeta))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	snowballK := 10
	snowball := NewSnowball()

	// Case 1: check that Snowball terminates properly given clear majority.

	var value = "vote_value"
	// A vote with clear majority.
	majorityVoteIndex := 0
	majorityVote := newTestVote(majorityVoteIndex, float64(snowballK), getRandomID(t), &value)

	var votes []Vote
	for i := 0; i < snowballK; i++ {
		if i == majorityVoteIndex {
			votes = append(votes, majorityVote)
		} else {
			votes = append(votes, newTestVote(i, 1, getRandomID(t), &value))
		}
	}

	assert.Nil(t, snowball.Preferred())
	assert.False(t, snowball.Decided())
	assert.Zero(t, snowball.Progress())

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := snowball.Preferred().(*testVote)
		assert.Equal(t, *majorityVote, *preferred)
	}

	assert.NotNil(t, snowball.Preferred())
	assert.Equal(t, &value, snowball.Preferred().Value().(*string))
	assert.True(t, snowball.Decided())
	assert.Equal(t, snowballBeta+1, snowball.count)
	assert.Len(t, snowball.counts, 1)

	// Try tick once more. Does absolutely nothing.
	cloned := *snowball // nolint:govet
	snowball.Tick(votes)
	assert.Equal(t, *snowball, cloned) // nolint:govet

	// Reset Snowball and assert everything is cleared properly.
	snowball.Reset()
	assert.False(t, snowball.Decided())
	assert.Nil(t, snowball.Preferred())
	assert.Nil(t, snowball.last)
	assert.Equal(t, 0, snowball.count)
	assert.Len(t, snowball.counts, 0)

	// Case 2: check that Snowball terminates properly given unanimous sampling of Round A, with preference
	// first initially to check for off-by-one errors.

	snowball.Prefer(majorityVote)

	preferred := snowball.Preferred().(*testVote)
	assert.Equal(t, *majorityVote, *preferred)

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := snowball.Preferred().(*testVote)
		assert.Equal(t, *majorityVote, *preferred)
	}

	assert.NotNil(t, snowball.Preferred())
	assert.Equal(t, &value, snowball.Preferred().Value().(*string))
	assert.True(t, snowball.Decided())
	assert.Equal(t, snowballBeta+1, snowball.count)
	assert.Len(t, snowball.counts, 1)

	// Reset Snowball and assert everything is cleared properly.
	snowball.Reset()
	assert.False(t, snowball.Decided())
	assert.Nil(t, snowball.Preferred())
	assert.Nil(t, snowball.last)
	assert.Equal(t, 0, snowball.count)
	assert.Len(t, snowball.counts, 0)

	// Case 3: check that Snowball terminates if we sample 10 times majority vote A, then sample 11 times majority vote B.
	// This demonstrates the that we need a large amount of samplings to overthrow our undecided preferred
	// vote, originally being A, such that it is B.

	for i := 0; i < snowballBeta; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := snowball.Preferred().(*testVote)
		assert.Equal(t, *majorityVote, *preferred)
	}

	assert.False(t, snowball.Decided())

	// Choose a random vote for the new majority.
	newMajorityVote := votes[5].(*testVote)
	// Set the tally to be higher than the tally of previous majority vote.
	newMajorityVote.SetTally(majorityVote.Tally() + 1)

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := snowball.Preferred().(*testVote)

		if i == snowballBeta {
			assert.Equal(t, *newMajorityVote, *preferred)
		} else {
			assert.Equal(t, *majorityVote, *preferred)

		}
	}

	// Reset snowball and tally
	snowball.Reset()
	newMajorityVote.SetTally(1)

	// Case 4: Check that it is impossible to overthrow decided preferred.

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := snowball.Preferred().(*testVote)
		assert.Equal(t, *majorityVote, *preferred)
	}

	assert.True(t, snowball.Decided())

	newMajorityVote.SetTally(majorityVote.Tally() + 1)
	for i := 0; i < snowballBeta*2; i++ {
		assert.True(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := snowball.Preferred().(*testVote)
		assert.Equal(t, *majorityVote, *preferred)
	}

	// Reset snowball and tally
	snowball.Reset()
	newMajorityVote.SetTally(1)

	// Case 5: tick with nil slice and empty slice when the snowball has prefer. Does nothing.

	snowball.Prefer(majorityVote)

	assert.Equal(t, &value, snowball.Preferred().Value().(*string))
	assert.False(t, snowball.Decided())
	assert.Len(t, snowball.counts, 0)
	assert.Equal(t, 0, snowball.count)
	assert.Nil(t, snowball.last)

	snowball.Tick(nil)
	snowball.Tick([]Vote{})

	assert.Equal(t, &value, snowball.Preferred().Value().(*string))
	assert.False(t, snowball.Decided())
	assert.Len(t, snowball.counts, 0)
	assert.Equal(t, 0, snowball.count)
	assert.Nil(t, snowball.last)

	snowball.Reset()

	// Case 6: tick with nil slice and empty slice when the snowball is empty. Does nothing.

	assert.Nil(t, snowball.Preferred())
	assert.False(t, snowball.Decided())
	assert.Len(t, snowball.counts, 0)
	assert.Equal(t, 0, snowball.count)
	assert.Nil(t, snowball.last)

	snowball.Tick(nil)
	snowball.Tick([]Vote{})

	assert.Nil(t, snowball.Preferred())
	assert.False(t, snowball.Decided())
	assert.Len(t, snowball.counts, 0)
	assert.Equal(t, 0, snowball.count)
	assert.Nil(t, snowball.last)
}

// Test if all tallies have equal value and the value is lower than alpha.
// Snowball will never decide.
func TestSnowball_EqualTally_LowerThanAlpha(t *testing.T) {
	t.Parallel()
	snowballBeta := 10
	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(snowballBeta))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	snowballK := 10
	snowball := NewSnowball()

	var votes []Vote
	for i := 0; i < snowballK; i++ {
		votes = append(votes, newTestVote(i, 0.1, getRandomID(t), nil))
	}

	// Case 1: the snowball does not have preferred.
	// Expected: The snowball will never have preferred.

	for i := 0; i < snowballBeta*2; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)

		assert.Nil(t, snowball.Preferred())
	}

	assert.False(t, snowball.Decided())
	assert.Nil(t, snowball.Preferred())

	snowball.Reset()

	// Case 1: the snowball already has preferred.
	// Expected: The preferred will not change.

	// Prefer a random vote
	last := votes[mrand.Intn(len(votes))].(*testVote)
	snowball.Prefer(last)

	for i := 0; i < snowballBeta*2; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)

		assert.Equal(t, *last, *snowball.Preferred().(*testVote))
	}

	assert.False(t, snowball.Decided())
	assert.Equal(t, *last, *snowball.Preferred().(*testVote))
}

// Test if all tallies have equal value and the value is higher than alpha.
// This is impossible because of the way the tally calculation works, total sum of the tallies will equal to 1.
//
// Nevertheless, because of the way the snowball is implemented, if this happens, the snowball will prefer the first vote.
func TestSnowball_EqualTally_HigherThanAlpha(t *testing.T) {
	t.Parallel()
	snowballBeta := 10
	defaultBeta := conf.GetSnowballBeta()
	conf.Update(conf.WithSnowballBeta(snowballBeta))
	defer func() {
		conf.Update(conf.WithSnowballBeta(defaultBeta))
	}()

	snowballK := 10
	snowball := NewSnowball()

	var votes []Vote
	for i := 0; i < snowballK; i++ {
		votes = append(votes, newTestVote(i, 0.2, getRandomID(t), nil))
	}
	firstVote := votes[0].(*testVote)

	// Case 1: the snowball does not have preferred.
	// The snowball should prefer the first vote.

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())

		snowball.Tick(votes)
		assert.Equal(t, *firstVote, *snowball.Preferred().(*testVote))
	}

	assert.True(t, snowball.Decided())

	snowball.Reset()

	// Case 2: the snowball already has preferred.
	// The snowball should prefer the first vote.

	last := votes[len(votes)-1].(*testVote)
	snowball.Prefer(last)

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		assert.Equal(t, *firstVote, *snowball.Preferred().(*testVote))
	}
	assert.True(t, snowball.Decided())
}
