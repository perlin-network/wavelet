package wavelet

import (
	"encoding/hex"
	"encoding/json"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"time"
)

const defaultGenesis = `
{
  "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405": {
    "balance": 100000000
  }
}
`

// performInception loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func performInception(accounts accounts, path string) (*Transaction, error) {
	file, _ := os.Open(path)

	defer func() {
		if file != nil {
			if err := file.Close(); err != nil {
				panic(err)
			}
		}
	}()

	var buf []byte
	var err error

	if file != nil {
		buf, err = ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}
	} else {
		buf = []byte(defaultGenesis)
	}

	var entries map[string]map[string]interface{}
	if err := json.Unmarshal(buf, &entries); err != nil {
		return nil, err
	}

	for encodedID, pairs := range entries {
		encodedIDBuf, err := hex.DecodeString(encodedID)

		if err != nil {
			return nil, err
		}

		var id common.AccountID
		copy(id[:], encodedIDBuf)

		for key, val := range pairs {
			switch key {
			case "balance":
				balance, ok := val.(float64)
				if !ok {
					return nil, errors.Errorf("failed to cast type for key %q with value %+v", key, val)
				}

				accounts.WriteAccountBalance(id, uint64(balance))
			case "stake":
				stake, ok := val.(float64)
				if !ok {
					return nil, errors.Errorf("failed to cast type for key %q with value %+v", key, val)
				}

				accounts.WriteAccountStake(id, uint64(stake))
			}
		}
	}

	// Commit all genesis changes to the ledger.
	err = accounts.Commit()
	if err != nil {
		return nil, errors.Wrap(err, "failed to commit genesis changes to ledger")
	}

	merkleRoot := accounts.tree.Checksum()

	// Spawn a genesis transaction.
	inception := time.Date(2018, time.Month(4), 26, 0, 0, 0, 0, time.UTC)

	tx := &Transaction{
		Timestamp:          uint64(time.Duration(inception.UnixNano()) / time.Millisecond),
		AccountsMerkleRoot: merkleRoot,
	}
	tx.rehash()

	return tx, nil
}
