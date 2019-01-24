package wavelet

import (
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/wavelet/log"
	"sync"
)

var (
	KeyWalletNonce = writeBytes("wallet_nonce_")
)

type Wallet struct {
	sync.Mutex
	*crypto.KeyPair
}

func NewWallet(keys *crypto.KeyPair) *Wallet {
	log.Info().
		Str("public_key", keys.PublicKeyHex()).
		Str("private_key", keys.PrivateKeyHex()).
		Msg("Keypair loaded.")

	wallet := &Wallet{KeyPair: keys}

	return wallet
}

// CurrentNonce returns our node's knowledge of our wallets current nonce.
func (w *Wallet) CurrentNonce(l *Ledger) uint64 {
	w.Lock()
	defer w.Unlock()

	walletNonceKey := merge(KeyWalletNonce, w.PublicKey)

	bytes, _ := l.Store.Get(walletNonceKey)
	if len(bytes) == 0 {
		bytes = make([]byte, 8)
	}
	return readUint64(bytes)
}

// NextNonce returns the next available nonce from the wallet.
func (w *Wallet) NextNonce(ledger *Ledger) (uint64, error) {
	w.Lock()
	defer w.Unlock()

	walletNonceKey := merge(KeyWalletNonce, w.PublicKey)

	bytes, _ := ledger.Store.Get(walletNonceKey)
	if bytes == nil {
		bytes = make([]byte, 8)
	}

	nonce := readUint64(bytes)

	// If our personal nodes tracked nonce is smaller than the stored nonce in our ledger, update
	// our personal nodes nonce to be the stored nonce in the ledger.
	if account := LoadAccount(ledger.Accounts, w.PublicKey); account.GetNonce() > nonce {
		nonce = account.GetNonce()
	}

	err := ledger.Store.Put(walletNonceKey, writeUint64(nonce+1))
	if err != nil {
		return nonce, err
	}

	return nonce, nil
}

// GetBalance returns the current balance of the wallet.
func (w *Wallet) GetBalance(ledger *Ledger) uint64 {
	return LoadAccount(ledger.Accounts, w.PublicKey).GetBalance()
}

func (w *Wallet) GetStake(ledger *Ledger) uint64 {
	return LoadAccount(ledger.Accounts, w.PublicKey).GetStake()
}
