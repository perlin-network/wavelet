package keystore_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/perlin-network/wavelet/keystore"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
)

func TestEncryptedKey(t *testing.T) {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		t.Error(err)
	}
	enc, err := keystore.NewEncryptedKey(keys.PrivateKey(), "test")
	if err != nil {
		t.Error(err)
	}
	enc.WriteToFile()

	ek, err := keystore.ReadFromFile()
	if err != nil {
		t.Error(err)
	}
	key, err := ek.Decrypt("test")
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(keys.PrivateKey(), key) {
		t.Errorf("decrypted key does not match original key.\nGot: %s \n Want: %s", key, keys.PrivateKey())

	}

	fmt.Println(key)
}
