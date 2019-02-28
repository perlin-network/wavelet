package wavelet

import (
	"encoding/hex"
	"encoding/json"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"time"
)

// performInception loads data expected to exist at the birth of any node in this ledgers network.
// The data is fed in as .json.
func performInception(accounts accounts, path string) (*Transaction, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
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

		var id [PublicKeySize]byte
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
	err = accounts.CommitAccounts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to commit genesis changes to ledger")
	}

	merkleRoot := accounts.tree.Checksum()

	// Spawn a genesis transaction whose payload is the checksum of all of
	// our accounts data.
	inception := time.Date(2018, time.Month(4), 26, 0, 0, 0, 0, time.UTC)

	tx := &Transaction{
		Timestamp:          uint64(time.Duration(inception.UnixNano()) / time.Millisecond),
		AccountsMerkleRoot: merkleRoot,
	}
	tx.rehash()

	return tx, nil
}
