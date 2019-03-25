package wavelet

import (
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
)

type TransactionValidator func(snapshot *avl.Tree, tx Transaction) error

func ValidateNopTransaction(_ *avl.Tree, _ Transaction) error {
	return nil
}

func ValidateTransferTransaction(snapshot *avl.Tree, tx Transaction) error {
	reader := payload.NewReader(tx.Payload)

	var recipient common.AccountID

	recipientBuf, err := reader.ReadBytes()
	if err != nil {
		return errors.Wrap(err, "transfer: failed to decode recipient")
	}

	if len(recipientBuf) != common.SizeAccountID {
		return errors.Errorf("transfer: provided recipient is not %d bytes, but %d bytes instead", common.SizeAccountID, len(recipientBuf))
	}

	copy(recipient[:], recipientBuf)

	amount, err := reader.ReadUint64()
	if err != nil {
		return errors.Wrap(err, "transfer: failed to decode amount to transfer")
	}

	senderBalance, _ := ReadAccountBalance(snapshot, tx.Sender)

	if senderBalance < amount {
		return errors.Errorf("transfer: not enough balance, wanting %d PERLs", amount)
	}

	return nil
}

func ValidateStakeTransaction(snapshot *avl.Tree, tx Transaction) error {
	raw, err := payload.NewReader(tx.Payload).ReadUint64()

	if err != nil {
		return errors.Wrap(err, "stake: failed to decode stake delta amount")
	}

	balance, _ := ReadAccountBalance(snapshot, tx.Sender)
	stake, _ := ReadAccountStake(snapshot, tx.Sender)

	delta := int64(raw)

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

	return nil
}
