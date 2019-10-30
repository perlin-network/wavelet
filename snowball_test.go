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
	"encoding/binary"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"testing"
)

func newTestVote(id int, voter *skademlia.ID, tally float64, value interface{}) Vote {
	var voteID VoteID
	// To avoid conflict with ZeroVoteID, make sure the id is never zero.
	binary.BigEndian.PutUint16(voteID[:], uint16(id+1))

	return Vote{
		id:     voteID,
		voter:  voter,
		tally:  tally,
		length: 0,
		value:  value,
	}
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
	majorityIndex := 0
	majority := newTestVote(majorityIndex, getRandomID(t), float64(snowballK), &value)

	var votes []Vote
	for i := 0; i < snowballK; i++ {
		if i == majorityIndex {
			votes = append(votes, majority)
		} else {
			value := fmt.Sprintf("vote_value_%d", i)
			votes = append(votes, newTestVote(i, getRandomID(t), 1, &value))
		}
	}

	assert.Nil(t, snowball.Preferred())
	assert.False(t, snowball.Decided())
	assert.Zero(t, snowball.Progress())

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := *snowball.Preferred()
		expected := majority

		assert.Equal(t, expected, preferred)
	}

	assert.NotNil(t, snowball.Preferred())
	assert.Equal(t, &value, snowball.Preferred().Value().(*string))
	assert.True(t, snowball.Decided())
	assert.Equal(t, snowballBeta+1, snowball.count)
	assert.Len(t, snowball.counts, 1)

	// Try tick once more. Does absolutely nothing.
	cloned := *snowball
	snowball.Tick(votes)
	assert.Equal(t, *snowball, cloned)

	// Reset Snowball and assert everything is cleared properly.
	snowball.Reset()
	assert.False(t, snowball.Decided())
	assert.Nil(t, snowball.Preferred())
	assert.Nil(t, snowball.last)
	assert.Equal(t, 0, snowball.count)
	assert.Len(t, snowball.counts, 0)

	// Case 2: check that Snowball terminates properly given unanimous sampling of Round A, with preference
	// first initially to check for off-by-one errors.

	snowball.Prefer(majority)

	preferred := *snowball.Preferred()
	assert.Equal(t, majority, preferred)

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := *snowball.Preferred()
		assert.Equal(t, majority, preferred)
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
		preferred := *snowball.Preferred()
		assert.Equal(t, majority, preferred)
	}

	assert.False(t, snowball.Decided())

	// Choose a random vote for the new majority.
	newMajority := &votes[5]
	// Set the tally to be higher than the tally of previous majority vote.
	newMajority.tally = majority.tally + 1

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := *snowball.Preferred()

		if i == snowballBeta {
			assert.Equal(t, *newMajority, preferred)
		} else {
			assert.Equal(t, majority, preferred)
		}
	}

	// Reset snowball and tally
	snowball.Reset()
	newMajority.tally = 1

	// Case 4: Check that it is impossible to overthrow decided preferred.

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := *snowball.Preferred()
		assert.Equal(t, majority, preferred)
	}

	assert.True(t, snowball.Decided())

	newMajority.tally = majority.tally + 1
	for i := 0; i < snowballBeta*2; i++ {
		assert.True(t, snowball.Decided())
		snowball.Tick(votes)
		preferred := *snowball.Preferred()
		assert.Equal(t, majority, preferred)
	}

	// Reset snowball and tally
	snowball.Reset()
	newMajority.tally = 1

	// Case 5: tick with nil slice and empty slice when the snowball has prefer. Does nothing.

	snowball.Prefer(majority)

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
		votes = append(votes, newTestVote(i, getRandomID(t), 0.1, nil))
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
	last := votes[rand.Intn(len(votes))]
	snowball.Prefer(last)

	for i := 0; i < snowballBeta*2; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)

		assert.Equal(t, last, *snowball.Preferred())
	}

	assert.False(t, snowball.Decided())
	assert.Equal(t, last, *snowball.Preferred())
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
		votes = append(votes, newTestVote(i, getRandomID(t), 0.2, nil))
	}
	firstVote := votes[0]

	// Case 1: the snowball does not have preferred.
	// The snowball should prefer the first vote.

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())

		snowball.Tick(votes)
		assert.Equal(t, firstVote, *snowball.Preferred())
	}

	assert.True(t, snowball.Decided())

	snowball.Reset()

	// Case 2: the snowball already has preferred.
	// The snowball should prefer the first vote.

	last := votes[len(votes)-1]
	snowball.Prefer(last)

	for i := 0; i < snowballBeta+1; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(votes)
		assert.Equal(t, firstVote, *snowball.Preferred())
	}
	assert.True(t, snowball.Decided())
}

func TestStruct(t *testing.T) {
	var responses []Vote
	responses = append(responses, Vote{
		id:     VoteID{},
		voter:  nil,
		length: 0,
		value:  nil,
		tally:  0,
	})

	responses = append(responses, Vote{
		id:     VoteID{},
		voter:  nil,
		length: 0,
		value:  nil,
		tally:  0,
	})

	votes := make(map[VoteID]Vote, len(responses))

	for _, res := range responses {
		vote, exists := votes[res.ID()]
		if !exists {
			vote = res
		}

		vote.tally += 1.0 / float64(len(responses))

		votes[vote.ID()] = vote
		//vote.SetTally(vote.Tally() + 1.0/float64(len(responses)))
	}

	for _, v := range votes {
		t.Log(v.tally)
	}
}
