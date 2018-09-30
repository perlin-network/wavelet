package wavelet

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadGenesisTransaction(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		filename   string
		publicKeys []string
		balances   []uint64
	}{
		{"config_1.csv",
			[]string{
				"f8cab2617bdd3127d1ba17f5c4890466c2c668610fa09a8416e9aeafdd8336c3",
				"ef999332ca9f567221a31549db23241e624d9e30f9a0c788f53cb5ded5c6d047",
			},
			[]uint64{
				1337,
				1000000,
			},
		},
	}

	for _, tt := range testCases {
		filepath := filepath.Join("test", "genesis", tt.filename)
		accounts, err := LoadGenesisTransaction(filepath)
		assert.Equal(t, nil, err, "%+v", err)
		for i, key := range tt.publicKeys {
			assert.Equalf(t, key, accounts[i].PublicKeyHex(), "public key should be equal i=%d", i)
		}
		for i, expectedBalance := range tt.balances {
			v, loaded := accounts[i].Load("balance")
			assert.Equalf(t, true, loaded, "balance should exists i=%d", i)
			assert.Equalf(t, expectedBalance, readUint64(v), "balance should be equal i=%d", i)
		}
	}
}
