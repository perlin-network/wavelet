package keystore_test

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/keystore"
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
	err = keystore.Write("enc.json", enc)
	if err != nil {
		t.Error(err)
	}

	// ek, err := keystore.ReadFromFile()
	// if err != nil {
	// 	t.Error(err)
	// }
	// key, err := ek.Decrypt("test")
	// if err != nil {
	// 	t.Error(err)
	// }

	// if !reflect.DeepEqual(keys.PrivateKey(), key) {
	// 	t.Errorf("decrypted key does not match original key.\nGot: %s \n Want: %s", key, keys.PrivateKey())

	// }

	// fmt.Println(key)
}

func TestPlainTextKeys(t *testing.T) {
	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		t.Error(err)
	}
	pt := keystore.NewPlainTextKey(keys.PrivateKey())

	err = keystore.Write("pt.json", pt)
	if err != nil {
		t.Error(err)
	}

	fi, err := os.Stat("pt.json")
	if err != nil {
		t.Error(err)
	}
	// get the size
	fmt.Println(fi.Size())

	privKey, err := pt.ExtractFromPlainTextKey()
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(keys.PrivateKey(), *privKey) {
		t.Errorf("decrypted key does not match original key.\nGot: %s \n Want: %s", *privKey, keys.PrivateKey())
	}
}

// func TestReadFromFile(t *testing.T) {
// 	_, err := keystore.ReadFromFile()
// 	if err != nil {
// 		t.Error(err.Error())
// 	}

// }
