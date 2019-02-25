package conflict

import (
	"golang.org/x/crypto/blake2b"
)

const (
	SnowballDefaultK     = 1
	SnowballDefaultAlpha = 0.8
	SnowballDefaultBeta  = 10
)

var _ Resolver = (*snowball)(nil)

type snowball struct {
	k, beta int
	alpha   float32

	preferred, last [blake2b.Size256]byte

	counts map[[blake2b.Size256]byte]int
	count  int

	decided bool
}

func NewSnowball() *snowball {
	return &snowball{
		k:     SnowballDefaultK,
		beta:  SnowballDefaultBeta,
		alpha: SnowballDefaultAlpha,

		counts: make(map[[blake2b.Size256]byte]int),
	}
}

func (s *snowball) WithK(k int) *snowball {
	s.k = k
	return s
}

func (s *snowball) WithAlpha(alpha float32) *snowball {
	s.alpha = alpha
	return s
}

func (s *snowball) WithBeta(beta int) *snowball {
	s.beta = beta
	return s
}

func (c *snowball) Tick(id [blake2b.Size256]byte, votes []bool) {
	tally := make(map[bool]int)

	for _, vote := range votes {
		tally[vote]++

		if tally[true] >= int(float32(c.k)*c.alpha) {
			c.counts[id]++

			if c.counts[id] > c.counts[c.preferred] {
				c.preferred = id
			}

			if c.last != id {
				c.last = id
				c.count = 0
			} else {
				c.count++

				if c.count > c.beta {
					c.decided = true
				}
			}

			break
		}
	}
}
