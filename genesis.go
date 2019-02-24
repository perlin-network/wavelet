package wavelet

import (
	"encoding/hex"
	"encoding/json"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
)

// LoadGenesis loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func LoadGenesis(path string) (*Transaction, error) {
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

	for encodedID, pairs := range entries {
		_, err := hex.DecodeString(encodedID)

		if err != nil {
			return nil, err
		}

		for key, val := range pairs {
			if key == "balance" {
				_, ok := val.(uint64)
				if !ok {
					return nil, errors.Errorf("failed to cast type for key 'balance' with value %+v", key, val)
				}

			}
		}
	}

	return nil, nil
}
