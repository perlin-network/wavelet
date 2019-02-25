package wavelet

import (
	"github.com/perlin-network/wavelet/sys"
	"golang.org/x/crypto/blake2b"
)

type snowballConflictResolver struct {
	preferred, last [blake2b.Size256]byte

	counts map[[blake2b.Size256]byte]int
	count  int

	decided bool
}

func newSnowballConflictResolver() *snowballConflictResolver {
	return &snowballConflictResolver{counts: make(map[[blake2b.Size256]byte]int)}
}

func (c *snowballConflictResolver) Tick(id [blake2b.Size256]byte, votes [sys.SnowballK]bool) {
	tally := make(map[bool]int)

	for _, vote := range votes {
		tally[vote]++

		if tally[true] >= int(float32(sys.SnowballK)*sys.SnowballAlpha) {
			c.counts[id]++

			if c.counts[id] > c.counts[c.preferred] {
				c.preferred = id
			}

			if c.last != id {
				c.last = id
				c.count = 0
			} else {
				c.count++

				if c.count > sys.SnowballThreshold {
					c.decided = true
				}
			}

			break
		}
	}
}
