EncryptedKey contains details about the Cipher / KDF parameters,
the encrypted key (cipher text), and account details for easy
tracking.


```golang
type EncryptedKey struct {
	Account     string    `json:"account"`
	Version     int       `json:"version"`
	TimeCreated time.Time `json:"created"`
	Crypto      Crypto    `json:"crypto"`
}
```


Crypto contains all of the parameters for the cipher and KDF.

```golang
type Crypto struct {
	Cipher       string      `json:"cipher"`
	CipherText   string      `json:"cipherText"`
	CipherParams interface{} `json:"cipherParams"`
	KDF          string      `json:"kdf"`
	KDFParams    interface{} `json:"kdfParams"`
}
```


Currently the only supported KDF is scrypt. The only supported enc is secretbox


```golang
// ScryptParams ...
type ScryptParams struct {
	N      int    `json:"n"`
	P      int    `json:"p"`
	R      int    `json:"r"`
	KeyLen int    `json:"keyLen"`
	Salt   string `json:"salt"`
}

// SecretboxParams ...
type SecretboxParams struct {
	Nonce string `json:"nonce"`
}

```

example keyfile: 


```json

{
  "account": "7987ec76b9dd351154e6a09088fa3c4f0a2839e10e416943d0490b3ecb52a2f6",
  "version": 1,
  "created": "2019-05-27T15:50:01.326585-05:00",
  "crypto": {
    "cipher": "secretbox",
    "cipherText": "75990fd3f2afcfe2b96c66d8a83860f870f7e84f106a1987a5850bf84a5978b8351b8f0119d765f11e31a04bd5dfbc385ea572ad4bf357c1c0a165fd8ae1caa3691bce8936390338908116d6356b4bcb",
    "cipherParams": {
      "nonce": "2799f35d93c753ae023a50ed395c00a7be998788a9a8d364"
    },
    "kdf": "scrypt",
    "kdfParams": {
      "n": 262144,
      "p": 1,
      "r": 8,
      "keyLen": 32,
      "salt": "529a737397e4d128e61db485dfff0c712e59c28f3e4d7aa2e5c202da3bf88cc4"
    }
  }
}

```


How it works:

1) password is hashed with scrypt to create a 32 byte key
2) derived key is used to encrypt the accounts private key
3) secretbox is authenticated hench no MAC