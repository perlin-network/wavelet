package wavelet

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/wavelet/log"
)

var (
	KeyWalletNonce = writeBytes("wallet_nonce_")
)

type Wallet struct {
	*crypto.KeyPair
	*database.Store
}

func NewWallet(keys *crypto.KeyPair, store *database.Store) *Wallet {
	log.Info().
		Str("public_key", keys.PublicKeyHex()).
		Str("private_key", keys.PrivateKeyHex()).
		Msg("Keypair loaded.")

	wallet := &Wallet{KeyPair: keys, Store: store}

	return wallet
}

// NextNonce returns the next available nonce from the wallet.
func (w *Wallet) NextNonce(ledger *Ledger) (uint64, error) {
	walletNonceKey := merge(KeyWalletNonce, w.PublicKey)

	bytes, _ := w.Get(walletNonceKey)
	if bytes == nil {
		bytes = make([]byte, 8)
	}

	nonce := readUint64(bytes)

	// If our personal nodes tracked nonce is smaller than the stored nonce in our ledger, update
	// our personal nodes nonce to be the stored nonce in the ledger.
	if account, err := ledger.LoadAccount(w.PublicKey); err == nil && account.Nonce > nonce {
		nonce = account.Nonce
	}

	err := w.Put(walletNonceKey, writeUint64(nonce+1))
	if err != nil {
		return nonce, err
	}

	return nonce, nil
}
