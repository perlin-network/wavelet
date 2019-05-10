package wavelet

import (
	"encoding/hex"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

const defaultGenesis = `
{
  "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405": {
    "balance": 10000000000000000000
  },
  "696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a": {
    "balance": 10000000000000000000
  }
}
`

// performInception loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func performInception(tree *avl.Tree, genesis *string) Round {
	var buf []byte

	if genesis != nil {
		buf = []byte(*genesis)
	} else {
		buf = []byte(defaultGenesis)
	}

	var p fastjson.Parser

	parsed, err := p.ParseBytes(buf)

	if err != nil {
		panic(err)
	}

	accounts, err := parsed.Object()

	if err != nil {
		panic(err)
	}

	var balance uint64
	var stake uint64

	accounts.Visit(func(key []byte, val *fastjson.Value) {
		if err != nil {
			return
		}

		var fields *fastjson.Object
		var id common.AccountID
		var n int

		n, err = hex.Decode(id[:], key)

		if n != cap(id) && err == nil {
			err = errors.Errorf("got an invalid account id: %s", key)
		}

		if err != nil {
			return
		}

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
			}
		})
	})

	if err != nil {
		panic(err)
	}

	tx := Transaction{}
	tx.rehash()

	return NewRound(0, tree.Checksum(), Transaction{}, tx)
}
