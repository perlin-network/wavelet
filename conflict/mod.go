package conflict

import (
	"golang.org/x/crypto/blake2b"
)

type Resolver interface {
	Reset()
	Tick(id [blake2b.Size256]byte, votes []float64)

	Result() [blake2b.Size256]byte
	Decided() bool
}
