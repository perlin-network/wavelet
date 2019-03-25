package wavelet

import (
	"encoding/hex"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	"io/ioutil"
	"os"
	"time"
)

const defaultGenesis = `
{
  "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405": {
    "balance": 10000000000000000000
  }
}
`

// performInception loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func performInception(tree *avl.Tree, path *string) (*Transaction, error) {
	var buf []byte

	if path != nil {
		file, err := os.Open(*path)

		if err != nil {
			return nil, err
		}

		defer func() {
			if err := file.Close(); err != nil {
				panic(err)
			}
		}()

		buf, err = ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}
	} else {
		buf = []byte(defaultGenesis)
	}

	var p fastjson.Parser

	parsed, err := p.ParseBytes(buf)

	if err != nil {
		return nil, err
	}

	accounts, err := parsed.Object()

	if err != nil {
		return nil, err
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
		return nil, err
	}

	merkleRoot := tree.Checksum()

	// Spawn a genesis transaction.
	inception := time.Date(2018, time.Month(4), 26, 0, 0, 0, 0, time.UTC)

	tx := &Transaction{
		Timestamp:          uint64(time.Duration(inception.UnixNano()) / time.Millisecond),
		AccountsMerkleRoot: merkleRoot,
	}
	tx.rehash()

	return tx, nil
}
