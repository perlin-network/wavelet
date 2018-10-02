package wavelet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type entry struct {
	PublicKey string `json:"public_key"`
	Balance   uint64 `json:"balance,omitempty"`
	Message   string `json:"message,omitempty"`
}

func TestLoadGenesisTransaction(t *testing.T) {
	t.Parallel()

	testCases := [][]entry{
		[]entry{
			{PublicKey: "f8cab2617bdd3127d1ba17f5c4890466c2c668610fa09a8416e9aeafdd8336c3", Balance: 1337, Message: "one"},
			{PublicKey: "ef999332ca9f567221a31549db23241e624d9e30f9a0c788f53cb5ded5c6d047", Balance: 1000000, Message: "two"},
		},
		[]entry{
			{PublicKey: "f8cab2617bdd3127d1ba17f5c4890466c2c668610fa09a8416e9aeafdd8336c3", Message: "one"},
			{PublicKey: "ef999332ca9f567221a31549db23241e624d9e30f9a0c788f53cb5ded5c6d047", Balance: 1000000},
		},
	}

	for i, entries := range testCases {
		tmpfile, err := ioutil.TempFile("", fmt.Sprintf("genesis-%d", i))
		require.Nilf(t, err, "test case %d", i)
		defer os.Remove(tmpfile.Name())

		b, err := json.Marshal(entries)
		require.Nilf(t, err, "test case %d", i)
		tmpfile.Write(b)

		accounts, err := LoadGenesisTransaction(tmpfile.Name())
		assert.Nilf(t, err, "test case %d", i)
		assert.Equalf(t, len(entries), len(accounts), "test case %d", i)
		for j, e := range entries {
			key := e.PublicKey
			balance := e.Balance
			message := e.Message
			assert.Equalf(t, key, accounts[j].PublicKeyHex(), "public key should be equal i=%d j=%d", i, j)
			b, loaded := accounts[j].Load("balance")
			if balance != 0 {
				assert.Equalf(t, true, loaded, "balance should exists i=%d j=%d", i, j)
				assert.Equalf(t, balance, readUint64(b), "balance should be equal i=%d j=%d", i, j)
			} else {
				assert.Equalf(t, false, loaded, "balance should not exists i=%d j=%d", i, j)
			}
			m, loaded := accounts[j].Load("message")
			if message != "" {
				assert.Equalf(t, true, loaded, "message should exists i=%d j=%d", i, j)
				assert.Equalf(t, message, string(m), "message should be equal i=%d j=%d", i, j)
			} else {
				assert.Equalf(t, false, loaded, "message should not exists i=%d j=%d", i, j)
			}
		}
	}
}
func TestLoadGenesisTransactionBadIDs(t *testing.T) {
	t.Parallel()

	testCases := [][]entry{
		[]entry{
			{PublicKey: "bad_id1", Balance: 1000000, Message: "one"},
			{PublicKey: "bad_id2"},
		},
	}

	for i, entries := range testCases {
		tmpfile, err := ioutil.TempFile("", fmt.Sprintf("genesis-%d", i))
		require.Nilf(t, err, "test case %d", i)
		defer os.Remove(tmpfile.Name())

		b, err := json.Marshal(entries)
		require.Nilf(t, err, "test case %d", i)
		tmpfile.Write(b)

		_, err = LoadGenesisTransaction(tmpfile.Name())
		assert.NotNilf(t, err, "test case %d", i)
	}
}
