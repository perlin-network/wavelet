package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"io"
)

type TransactionValidator func(snapshot *avl.Tree, tx Transaction) error

func ValidateNopTransaction(_ *avl.Tree, _ Transaction) error {
	return nil
}

func ValidateTransferTransaction(snapshot *avl.Tree, tx Transaction) error {
	r := bytes.NewReader(tx.Payload)

	var recipient common.AccountID

	if _, err := io.ReadFull(r, recipient[:]); err != nil {
		return errors.Wrap(err, "transfer: failed to decode recipient")
	}

	var buf [8]byte

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "transfer: failed to decode amount to transfer")
	}

	amount := binary.LittleEndian.Uint64(buf[:])

	creatorBalance, _ := ReadAccountBalance(snapshot, tx.Creator)

	if creatorBalance < amount {
		return errors.Errorf("transfer: transaction creator tried to send %d PERLs, but only has %d PERLs", amount, creatorBalance)
	}

	return nil
}

func ValidateStakeTransaction(snapshot *avl.Tree, tx Transaction) error {
	r := bytes.NewReader(tx.Payload)

	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "stake: failed to decode amount of stake to place/withdraw")
	}

	delta := int64(binary.LittleEndian.Uint64(buf[:]))

	balance, _ := ReadAccountBalance(snapshot, tx.Creator)
	stake, _ := ReadAccountStake(snapshot, tx.Creator)

	if delta >= 0 {
		delta := uint64(delta)

		if balance < delta {
			return errors.New("stake: balance < delta")
		}
	} else {
		delta := uint64(-delta)

		if stake < delta {
			return errors.New("stake: stake < delta")
		}
	}

	return nil
}

func ValidateContractTransaction(snapshot *avl.Tree, tx Transaction) error {
	if len(tx.Payload) == 0 {
		return errors.New("contract: no code specified for contract to-be-spawned")
	}

	if _, exists := ReadAccountContractCode(snapshot, tx.ID); exists {
		return errors.New("contract: there already exists a contract spawned with the specified code")
	}

	executor := NewContractExecutor(tx.ID, nil).WithGasTable(sys.GasTable)

	if _, err := executor.Init(tx.Payload, 50000000); err != nil {
		return errors.New("contract: code for contract is not valid WebAssembly code")
	}

	return nil
}
