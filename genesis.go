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
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

const testingGenesis = `
{
  "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405": {
    "balance": 10000000000000000000,
    "reward": 5000000
  },
  "696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a": {
    "balance": 10000000000000000000
  },
  "f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467": {
    "balance": 10000000000000000000
  }
}
`

const testnetGenesis = `
{
    "0f569c84d434fb0ca682c733176f7c0c2d853fce04d95ae131d2f9b4124d93d8": {
        "balance": 10000000000000000000
    }
}
`

var defaultGenesis = testingGenesis

func SetGenesisByNetwork(name string) error {
	switch name {
	case "testnet":
		defaultGenesis = testnetGenesis
	case "testing":
		defaultGenesis = testingGenesis
	default:
		return fmt.Errorf("Invalid network: %s", name)
	}

	return nil
}

// performInception loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func performInception(tree *avl.Tree, genesis *string) Block {
	logger := log.Node()

	var buf []byte

	if genesis != nil {
		buf = []byte(*genesis)
	} else {
		buf = []byte(defaultGenesis)
	}

	var p fastjson.Parser

	parsed, err := p.ParseBytes(buf)

	if err != nil {
		logger.Fatal().Err(err).Msg("ParseBytes()")
	}

	accounts, err := parsed.Object()

	if err != nil {
		logger.Fatal().Err(err).Msg("parsed.Object()")
	}

	var balance, stake, reward uint64

	set := make(map[AccountID]struct{}) // Ensure that there are no duplicate account entries in the JSON.

	accounts.Visit(func(key []byte, val *fastjson.Value) {
		if err != nil {
			return
		}

		var fields *fastjson.Object
		var id AccountID
		var n int

		n, err = hex.Decode(id[:], key)

		if n != cap(id) && err == nil {
			err = errors.Errorf("got an invalid account ID: %x", key)
			return
		}

		if err != nil {
			err = errors.Wrapf(err, "got an invalid account ID: %x", key)
			return
		}

		if _, exists := set[id]; exists {
			err = errors.Errorf("found duplicate entries for account ID %x in genesis file", id)
			return
		}

		set[id] = struct{}{}

		fields, err = val.Object()

		if err != nil {
			return
		}

		fields.Visit(func(key []byte, v *fastjson.Value) {
			if err != nil {
				return
			}

			switch string(key) {
			case "balance":
				balance, err = v.Uint64()
				if err != nil {
					err = errors.Wrapf(err, "failed to cast type for key %q", key)
					return
				}

				WriteAccountBalance(tree, id, balance)
			case "stake":
				stake, err = v.Uint64()

				if err != nil {
					err = errors.Wrapf(err, "failed to cast type for key %q", key)
					return
				}

				WriteAccountStake(tree, id, uint64(stake))
			case "reward":
				reward, err = v.Uint64()

				if err != nil {
					err = errors.Wrapf(err, "failed to cast type for key %q", key)
					return
				}

				WriteAccountReward(tree, id, uint64(reward))
			}
		})

		if err == nil {
			WriteAccountsLen(tree, ReadAccountsLen(tree)+1)
			WriteAccountNonce(tree, id, 1)
		}
	})

	if err != nil {
		logger.Fatal().Err(err).Msg("accounts.Visit")
	}

	tx := Transaction{}
	tx.rehash()

	return NewBlock(0, tree.Checksum(), []BlockID{}...)
}
