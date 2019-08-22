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
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewSnowball(t *testing.T) {
	t.Parallel()

	snowball := NewSnowball(WithBeta(10))

	keys, err := skademlia.NewKeys(1, 1)
	assert.NoError(t, err)

	start := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagTransfer, nil))

	endA := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagStake, nil))
	endB := AttachSenderToTransaction(keys, NewTransaction(keys, sys.TagContract, nil))

	a := NewRound(1, ZeroMerkleNodeID, 1337, start, endA)
	b := NewRound(1, ZeroMerkleNodeID, 1010, start, endB)

	// Check that Snowball terminates properly given unanimous sampling of Round A.

	assert.Nil(t, snowball.Preferred())

	var preferred *Round
	for i := 0; i < 12; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(&a)
		preferred = snowball.Preferred().(*Round)
		assert.Equal(t, *preferred, a)
	}

	assert.True(t, snowball.Decided())

	preferred = snowball.Preferred().(*Round)
	assert.Equal(t, *preferred, a)

	assert.Equal(t, snowball.count, 11)
	assert.Len(t, snowball.counts, 1)
	assert.Len(t, snowball.candidates, 1)

	// Try tick once more. Does absolutely nothing.

	cloned := *snowball
	snowball.Tick(&a)
	assert.Equal(t, cloned, *snowball)

	// Reset Snowball and assert everything is cleared properly.

	snowball.Reset()

	assert.False(t, snowball.Decided())
	assert.Nil(t, snowball.Preferred())

	assert.Equal(t, snowball.count, 0)
	assert.Len(t, snowball.counts, 0)
	assert.Len(t, snowball.candidates, 0)

	// Check that Snowball terminates properly given unanimous sampling of Round A, with preference
	// first initially to check for off-by-one errors.

	snowball.Prefer(&a)

	preferred = snowball.Preferred().(*Round)
	assert.Equal(t, *preferred, a)

	for i := 0; i < 12; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(&a)
		preferred = snowball.Preferred().(*Round)
		assert.Equal(t, *preferred, a)
	}

	assert.True(t, snowball.Decided())
	preferred = snowball.Preferred().(*Round)
	assert.Equal(t, *preferred, a)

	assert.Equal(t, snowball.count, 11)
	assert.Len(t, snowball.counts, 1)
	assert.Len(t, snowball.candidates, 1)

	// Reset Snowball and assert everything is cleared properly.

	snowball.Reset()

	assert.False(t, snowball.Decided())
	assert.Nil(t, snowball.Preferred())

	assert.Equal(t, snowball.count, 0)
	assert.Len(t, snowball.counts, 0)
	assert.Len(t, snowball.candidates, 0)

	// Check that Snowball terminates if we sample 11 times Round A, then sample 12 times Round B.
	// This demonstrates the that we need a large amount of samplings to overthrow our preferred
	// round, originally being A, such that it is B.

	for i := 0; i < 11; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(&a)
		preferred = snowball.Preferred().(*Round)
		assert.Equal(t, *preferred, a)
	}

	assert.False(t, snowball.Decided())

	for i := 0; i < 12; i++ {
		assert.False(t, snowball.Decided())
		snowball.Tick(&b)
		preferred = snowball.Preferred().(*Round)
		if i == 11 {
			assert.Equal(t, *preferred, b)
		} else {
			assert.Equal(t, *preferred, a)
		}
	}

	assert.Equal(t, snowball.counts[a.GetID()], 11)
	assert.Equal(t, snowball.counts[b.GetID()], 12)

	assert.True(t, snowball.Decided())

	preferred = snowball.Preferred().(*Round)
	assert.Equal(t, *preferred, b)
	assert.Equal(t, snowball.count, 11)
	assert.Len(t, snowball.counts, 2)
	assert.Len(t, snowball.candidates, 2)

	// Try cause a panic by ticking with nil, or with an empty round.

	empty := &Round{}

	snowball.Tick(nil)
	snowball.Tick(empty)

	assert.Equal(t, snowball.counts[a.GetID()], 11)
	assert.Equal(t, snowball.counts[b.GetID()], 12)

	assert.True(t, snowball.Decided())

	preferred = snowball.Preferred().(*Round)
	assert.Equal(t, b, *preferred)
	assert.Equal(t, 11, snowball.count)
	assert.Len(t, snowball.counts, 2)
	assert.Len(t, snowball.candidates, 2)

	// Try tick with nil if Snowball has not decided yet.

	snowball.Reset()

	snowball.Tick(&a)
	snowball.Tick(&a)

	assert.Equal(t, a.GetID(), snowball.lastID)
	assert.Equal(t, 1, snowball.Progress())
	assert.Len(t, snowball.counts, 1)

	snowball.Tick(nil)

	assert.Equal(t, "", snowball.lastID)
	assert.Equal(t, 0, snowball.Progress())
	assert.Len(t, snowball.counts, 1)
}
