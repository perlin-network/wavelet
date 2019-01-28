package wavelet

import (
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
)

// ReadGenesis loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func ReadGenesis(ledger *Ledger, path string) ([]*Account, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var entries map[string]map[string]interface{}
	if err := json.Unmarshal(bytes, &entries); err != nil {
		return nil, err
	}

	var genesis []*Account

	for encodedID, pairs := range entries {
		id, err := hex.DecodeString(encodedID)

		if err != nil {
			return nil, err
		}

		account := LoadAccount(ledger.Accounts, id)

		for key, v := range pairs {
			switch value := v.(type) {
			case float64:
				uintVal := uint64(value)
				account.Store(key, writeUint64(uintVal))
			case string:
				account.Store(key, writeBytes(value))
			default:
				return nil, errors.Errorf("failed to cast type for key %s with value %+v", key, value)
			}
		}

		genesis = append(genesis, account)
	}

	return genesis, nil
}
