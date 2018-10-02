package wavelet

import (
	"encoding/json"
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
			{"f8cab2617bdd3127d1ba17f5c4890466c2c668610fa09a8416e9aeafdd8336c3", 1337, "one"},
			{"ef999332ca9f567221a31549db23241e624d9e30f9a0c788f53cb5ded5c6d047", 1000000, "two"},
		},
	}

	for i, entries := range testCases {
		tmpfile, err := ioutil.TempFile("", "genesis")
		require.Nilf(t, err, "test case %d", i)
		defer os.Remove(tmpfile.Name())

		b, err := json.Marshal(entries)
		require.Nilf(t, err, "test case %d", i)
		tmpfile.Write(b)

		t.Logf("contents: %s", b)

		accounts, err := LoadGenesisTransaction(tmpfile.Name())
		assert.Nilf(t, err, "test case %d", i)
		assert.Equalf(t, len(entries), len(accounts), "test case %d", i)
		for j, e := range entries {
			key := e.PublicKey
			balance := e.Balance
			message := e.Message
			assert.Equalf(t, key, accounts[i].PublicKeyHex(), "public key should be equal i=%d j=%d", i, j)
			b, loaded := accounts[i].Load("balance")
			assert.Equalf(t, true, loaded, "balance should exists i=%d j=%d", i, j)
			assert.Equalf(t, balance, readUint64(b), "balance should be equal i=%d j=%d", i, j)
			m, loaded := accounts[i].Load("message")
			assert.Equalf(t, true, loaded, "message should exists i=%d j=%d", i, j)
			assert.Equalf(t, message, string(m), "message should be equal i=%d j=%d", i, j)
		}
	}
}
