package server

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/cmd/cli/tui/logger"
	"github.com/perlin-network/wavelet/sys"
)

func (s *Server) readKeys(wallet string) (*skademlia.Keypair, error) {
	var keys *skademlia.Keypair

	privateKeyBuf, err := ioutil.ReadFile(wallet)

	if err == nil {
		var privateKey edwards25519.PrivateKey

		n, err := hex.Decode(privateKey[:], privateKeyBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to decode your private key from %q", wallet)
		}

		if n != edwards25519.SizePrivateKey {
			return nil, fmt.Errorf("private key located in %q is not of the right length", wallet)
		}

		keys, err = skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
		if err != nil {
			return nil, fmt.Errorf("the private key specified in %q is invalid", wallet)
		}

		publicKey := keys.PublicKey()

		s.logger.Level(logger.WithSuccess("Wallet loaded.").
			F("privatekey", "%x", privateKey).
			F("publickey", "%x", publicKey))

		return keys, nil
	}

	if os.IsNotExist(err) {
		// If a private key is specified instead of a path to a wallet, then simply use the provided private key instead.
		if len(wallet) == hex.EncodedLen(edwards25519.SizePrivateKey) {
			var privateKey edwards25519.PrivateKey

			n, err := hex.Decode(privateKey[:], []byte(wallet))
			if err != nil {
				return nil, fmt.Errorf("failed to decode the private key specified: %s", wallet)
			}

			if n != edwards25519.SizePrivateKey {
				return nil, fmt.Errorf("private key %s is not of the right length", wallet)
			}

			keys, err = skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
			if err != nil {
				return nil, fmt.Errorf("the private key specified is invalid: %s", wallet)
			}

			publicKey := keys.PublicKey()

			s.logger.Level(logger.WithSuccess("A private key was provided instead of a wallet file.").
				F("privatekey", "%x", privateKey).
				F("publickey", "%x", publicKey))

			return keys, nil
		}

		keys, err = skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
		if err != nil {
			return nil, errors.New("failed to generate a new wallet")
		}

		privateKey := keys.PrivateKey()
		publicKey := keys.PublicKey()

		s.logger.Level(logger.WithSuccess("Existing wallet not found: generated a new one.").
			F("privatekey", "%x", privateKey).
			F("publickey", "%x", publicKey))

		return keys, nil
	}

	return keys, err
}
