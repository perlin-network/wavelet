package security

import (
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/crypto/blake2b"
	"github.com/perlin-network/noise/crypto/ed25519"
)

type KeyPair = crypto.KeyPair

var (
	HashPolicy      = blake2b.New()
	SignaturePolicy = ed25519.New()

	FromPrivateKey  = crypto.FromPrivateKey
	VerifySignature = crypto.Verify
	RandomKeyPair   = ed25519.RandomKeyPair
)

func Hash(message []byte) []byte {
	return HashPolicy.HashBytes(message)
}

func Sign(privateKey []byte, message []byte) []byte {
	return SignaturePolicy.Sign(privateKey, message)
}

func Verify(publicKey []byte, message []byte, signature []byte) bool {
	return SignaturePolicy.Verify(publicKey, message, signature)
}
