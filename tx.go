package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/avl"
	"github.com/phf/go-queue/queue"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"math/bits"
)

const PublicKeySize = 32
const SignatureSize = 64
const MaxTransactionPayloadSize = 1024 * 100

var _ noise.Message = (*Transaction)(nil)

type Transaction struct {
	// WIRE FORMAT
	ID [blake2b.Size256]byte

	Sender, Creator [PublicKeySize]byte

	ParentIDs [][blake2b.Size256]byte

	Timestamp uint64

	Tag byte

	Payload []byte

	// Only set if the transaction is a critical transaction.
	AccountsMerkleRoot [avl.MerkleRootSize]byte

	SenderSignature, CreatorSignature [SignatureSize]byte

	// IN-MEMORY DATA
	children [][blake2b.Size256]byte
	depth    uint64
}

func prefixLen(buf []byte) int {
	for i, b := range buf {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(uint8(b))
		}
	}

	return len(buf)*8 - 1
}

func (t *Transaction) IsCritical(difficulty int) bool {
	var buf bytes.Buffer
	_, _ = buf.Write(t.Sender[:])

	for _, parentID := range t.ParentIDs {
		_, _ = buf.Write(parentID[:])
	}

	checksum := blake2b.Sum256(buf.Bytes())

	return prefixLen(checksum[:]) >= difficulty
}

func (t *Transaction) Read(reader payload.Reader) (noise.Message, error) {
	n, err := reader.Read(t.Sender[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode transaction sender")
	}

	if n != PublicKeySize {
		return nil, errors.New("could not read enough bytes for transaction sender")
	}

	n, err = reader.Read(t.Creator[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode transaction creator")
	}

	if n != PublicKeySize {
		return nil, errors.New("could not read enough bytes for transaction creator")
	}

	numParents, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read num parents")
	}

	for i := 0; i < int(numParents); i++ {
		var parentID [PublicKeySize]byte

		n, err = reader.Read(parentID[:])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode parent %d", i)
		}

		if n != PublicKeySize {
			return nil, errors.Errorf("could not read enough bytes for parent %d", i)
		}

		t.ParentIDs = append(t.ParentIDs, parentID)
	}

	t.Timestamp, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction timestamp")
	}

	t.Tag, err = reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction tag")
	}

	t.Payload, err = reader.ReadBytes()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction payload")
	}

	if len(t.Payload) > MaxTransactionPayloadSize {
		return nil, errors.Errorf("transaction payload is of size %d, but can at most only handle %d bytes", len(t.Payload), MaxTransactionPayloadSize)
	}

	// If there exists an account merkle root, read it.
	if reader.Len() > SignatureSize*2 {
		n, err = reader.Read(t.AccountsMerkleRoot[:])
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode accounts merkle root")
		}

		if n != avl.MerkleRootSize {
			return nil, errors.New("could not read enough bytes for accounts merkle root")
		}
	}

	n, err = reader.Read(t.SenderSignature[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode sender signature")
	}

	if n != SignatureSize {
		return nil, errors.New("could not read enough bytes for sender signature")
	}

	n, err = reader.Read(t.CreatorSignature[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode creator signature")
	}

	if n != SignatureSize {
		return nil, errors.New("could not read enough bytes for creator signature")
	}

	t.rehash()

	return t, nil
}

func (t *Transaction) Write() []byte {
	writer := payload.NewWriter(nil)

	_, _ = writer.Write(t.Sender[:])
	_, _ = writer.Write(t.Creator[:])

	writer.WriteByte(byte(len(t.ParentIDs)))

	for _, parentID := range t.ParentIDs {
		_, _ = writer.Write(parentID[:])
	}

	writer.WriteUint64(t.Timestamp)
	writer.WriteByte(t.Tag)
	writer.WriteBytes(t.Payload)

	if prefixLen(t.AccountsMerkleRoot[:]) != 8*avl.MerkleRootSize {
		_, _ = writer.Write(t.AccountsMerkleRoot[:])
	}

	_, _ = writer.Write(t.SenderSignature[:])
	_, _ = writer.Write(t.CreatorSignature[:])

	return writer.Bytes()
}

func (t *Transaction) rehash() {
	t.ID = blake2b.Sum256(t.Write())
}

type TransactionProcessor interface {
	OnApplyTransaction(ctx *TransactionContext) error
}

type TransactionContext struct {
	accounts accounts

	balances map[[PublicKeySize]byte]uint64
	stakes   map[[PublicKeySize]byte]uint64

	transactions queue.Queue
	tx           *Transaction
}

func newTransactionContext(accounts accounts, tx *Transaction) *TransactionContext {
	ctx := &TransactionContext{
		accounts: accounts,
		balances: make(map[[PublicKeySize]byte]uint64),
		stakes:   make(map[[PublicKeySize]byte]uint64),

		tx: tx,
	}

	ctx.transactions.PushBack(tx)

	return ctx
}

func (c *TransactionContext) Transaction() Transaction {
	return *c.tx
}

func (c *TransactionContext) SendTransaction(tx *Transaction) {
	c.transactions.PushBack(tx)
}

func (c *TransactionContext) ReadAccountBalance(id [PublicKeySize]byte) (uint64, bool) {
	if balance, ok := c.balances[id]; ok {
		return balance, true
	}

	balance, exists := c.accounts.ReadAccountBalance(id)
	c.balances[id] = balance
	return balance, exists
}

func (c *TransactionContext) ReadAccountStake(id [PublicKeySize]byte) (uint64, bool) {
	if stake, ok := c.stakes[id]; ok {
		return stake, true
	}

	stake, exists := c.accounts.ReadAccountStake(id)
	c.stakes[id] = stake
	return stake, exists
}

func (c *TransactionContext) WriteAccountBalance(id [PublicKeySize]byte, balance uint64) {
	c.balances[id] = balance
}

func (c *TransactionContext) WriteAccountStake(id [PublicKeySize]byte, stake uint64) {
	c.stakes[id] = stake
}

func (c *TransactionContext) apply(processor TransactionProcessor) error {
	for c.transactions.Len() > 0 {
		c.tx = c.transactions.PopFront().(*Transaction)

		err := processor.OnApplyTransaction(c)
		if err != nil {
			return errors.Wrap(err, "failed to apply transaction")
		}
	}

	// If the transaction processor executed properly, apply changes from
	// the transactions context over to our accounts snapshot.

	for id, balance := range c.balances {
		c.accounts.WriteAccountBalance(id, balance)
	}

	for id, stake := range c.stakes {
		c.accounts.WriteAccountStake(id, stake)
	}

	return nil
}
