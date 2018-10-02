package wavelet

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"testing"
)

func check(t *testing.T, file *os.File, err error) {
	if err != nil {
		if file != nil {
			os.Remove(file.Name())
		}
		t.Fatal(err)
	}
}

func TestLoadGenesis(t *testing.T) {
	t.Parallel()

	cases := []map[string]map[string]interface{}{
		{
			"f8cab2617bdd3127d1ba17f5c4890466c2c668610fa09a8416e9aeafdd8336c3": {
				"balance": uint64(1337),
				"message": "one",
			},
			"ef999332ca9f567221a31549db23241e624d9e30f9a0c788f53cb5ded5c6d047": {
				"balance": uint64(1000000),
				"message": "two",
			},
		},
		{
			"f8cab2617bdd3127d1ba17f5c4890466c2c668610fa09a8416e9aeafdd8336c3": {
				"message": "one",
			},
			"ef999332ca9f567221a31549db23241e624d9e30f9a0c788f53cb5ded5c6d047": {
				"balance": uint64(1000000),
			},
		},
	}

	for i, template := range cases {
		tmp, err := ioutil.TempFile("", fmt.Sprintf("genesis-%d", i))
		check(t, tmp, err)

		bytes, err := json.Marshal(template)
		check(t, tmp, err)

		_, err = tmp.Write(bytes)
		check(t, tmp, err)

		genesis, err := ReadGenesis(tmp.Name())
		check(t, tmp, err)

		assert.Equalf(t, len(template), len(genesis), "test case %d", i)

		for targetID, pairs := range template {
			var target *Account

			for _, account := range genesis {
				if account.PublicKeyHex() == targetID {
					target = account
					break
				}
			}

			if target == nil {
				t.Fatalf("failed to find account %s in genesis", targetID)
			}

			balance, exists := pairs["balance"]
			loadedBalance, loaded := target.Load("balance")

			if loaded {
				assert.Equal(t, balance, readUint64(loadedBalance), "expected balances to be equal")
			}
			assert.Equal(t, loaded, exists, "expected balances to exist")

			message, exists := pairs["message"]
			loadedMessage, loaded := target.Load("message")

			if len(loadedMessage) > 0 {
				assert.Equal(t, message, string(loadedMessage), "expected messages to be equal")
			}
			assert.Equal(t, loaded, exists, "expected messages to exist")
		}

		os.Remove(tmp.Name())
	}
}

func TestLoadGenesisBadIDs(t *testing.T) {
	t.Parallel()

	cases := []map[string]map[string]interface{}{
		{
			"bad_id1": {
				"balance": uint64(1000000),
				"message": "one",
			},
		},
		{
			"bad_id2": {},
		},
	}

	for i, template := range cases {
		tmp, err := ioutil.TempFile("", fmt.Sprintf("genesis-%d", i))
		check(t, tmp, err)

		bytes, err := json.Marshal(template)
		check(t, tmp, err)

		_, err = tmp.Write(bytes)
		check(t, tmp, err)

		_, err = ReadGenesis(tmp.Name())
		assert.NotNil(t, err, "expected there to be an error loading invalid account IDs")

		os.Remove(tmp.Name())
	}
}
