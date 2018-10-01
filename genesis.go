package wavelet

import (
	"bufio"
	"encoding/csv"
	"encoding/hex"
	"io"
	"os"
	"strconv"

	"github.com/perlin-network/wavelet/log"
)

// LoadGenesisTransaction loads the genesis transaction from a csv file.
func LoadGenesisTransaction(path string) ([]*Account, error) {
	csvFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(bufio.NewReader(csvFile))
	var accounts []*Account
	var headers []string
	for i := 0; ; i++ {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if i == 0 {
			// read the headers only
			headers = line
			continue
		}

		// process data rows
		encoded := line[0]

		id, err := hex.DecodeString(encoded)
		if err != nil {
			return nil, err
		}

		account := NewAccount(id)
		for j := 1; j < len(headers); j++ {
			key := headers[j]
			v := line[j]

			// TODO: figure out how to get type information from the csv
			value, err := strconv.ParseUint(v, 0, 64)
			if err != nil {
				return nil, err
			}
			account.Store(key, writeUint64(value))
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// ApplyGenesisTransactions applies accounts to the ledger
func ApplyGenesisTransactions(ledger *Ledger, accounts []*Account) {
	for i, account := range accounts {
		if err := ledger.SaveAccount(account, nil); err != nil {
			log.Fatal().Err(err).
				Str("public_key", string(account.PublicKey)).
				Int("index", i).
				Msg("Failed to save genesis account information.")
		}
	}
}
