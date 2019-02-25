package conflict

import (
	"golang.org/x/crypto/blake2b"
)

type Resolver interface {
	Tick(id [blake2b.Size256]byte, votes []bool)
}
