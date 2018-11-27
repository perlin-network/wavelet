package security

import (
	"github.com/perlin-network/wavelet/params"

	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/crypto/ed25519"

	"github.com/pkg/errors"
)

var (
	ErrInvalidSignature     = errors.New("malformed signature")
	ErrMalformedTransaction = errors.New("malformed input")
)

// ValidateWiredTransaction validates an incoming transaction from our wire protocol
//
// Checks:
// Nop with empty payload.
// Sender public key length.
// Signature length.
// Tag length > 0.
// Valid sender public key.
// Valid message signature.
func ValidateWiredTransaction(wired *wire.Transaction) (bool, error) {
	if wired.Tag == params.TagNop && wired.Payload != nil {
		return false, errors.Wrap(ErrMalformedTransaction, "nop must have empty payload")
	}

	if len(wired.Sender) != ed25519.PublicKeySize {
		return false, errors.Wrap(ErrMalformedTransaction, "invalid sender id")
	}

	if len(wired.Signature) != ed25519.SignatureSize {
		return false, errors.Wrapf(ErrInvalidSignature, "invalid signature from sender %s", wired.Sender)
	}

	if len(wired.Tag) == 0 {
		return false, errors.Wrap(ErrMalformedTransaction, "invalid tag")
	}

	signature := wired.Signature
	wired.Signature = nil

	encoded, err := wired.Marshal()
	if err != nil {
		return false, err
	}

	if verified := Verify(wired.Sender, encoded, signature); !verified {
		return false, errors.Wrapf(ErrInvalidSignature, "invalid signature from sender %s", wired.Sender)
	}

	wired.Signature = signature

	return true, nil
}
