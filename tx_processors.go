package wavelet

import (
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

func ProcessNopTransaction(_ *TransactionContext) error {
	return nil
}

func ProcessTransferTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

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

	senderBalance, _ := ctx.ReadAccountBalance(tx.Sender)

	if senderBalance < amount {
		return errors.Errorf("transfer: not enough balance, wanting %d PERLs", amount)
	}

	ctx.WriteAccountBalance(tx.Sender, senderBalance-amount)

	recipientBalance, _ := ctx.ReadAccountBalance(recipient)
	ctx.WriteAccountBalance(recipient, recipientBalance+amount)

	if _, isContract := ctx.ReadAccountContractCode(recipient); !isContract {
		return nil
	}

	executor, err := NewContractExecutor(recipient, ctx)
	if err != nil {
		return errors.Wrap(err, "transfer: failed to load and init smart contract vm")
	}
	executor.WithGasTable(sys.GasTable)
	executor.EnableLogging()

	var gas uint64

	if reader.Len() > 0 {
		funcName, err := reader.ReadString()
		if err != nil {
			return errors.Wrap(err, "transfer: failed to read smart contract func name")
		}

		funcParams, err := reader.ReadBytes()
		if err != nil {
			return err
		}

		_, gas, err = executor.Run(amount, 50000000, funcName, funcParams...)
	} else {
		_, gas, err = executor.Run(amount, 50000000, "on_money_received")
	}

	if err != nil && errors.Cause(err) != ErrContractFunctionNotFound {
		return errors.Wrap(err, "transfer: failed to execute smart contract method")
	}

	if senderBalance-amount < gas {
		return errors.Errorf("transfer: sender only has %d PERLs, but must pay for gas fees which are %d PERLs", senderBalance-amount, gas)
	}

	ctx.WriteAccountBalance(tx.Sender, senderBalance-amount-gas)

	logger := log.Contract(recipient, "gas")
	logger.Info().
		Uint64("gas", gas).
		Hex("sender_id", tx.Sender[:]).
		Hex("contract_id", recipient[:]).
		Msg("Deducted PERLs for invoking smart contract function.")

	return nil
}

func ProcessStakeTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	raw, err := payload.NewReader(tx.Payload).ReadUint64()

	if err != nil {
		return errors.Wrap(err, "stake: failed to decode stake delta amount")
	}

	balance, _ := ctx.ReadAccountBalance(tx.Sender)
	stake, _ := ctx.ReadAccountStake(tx.Sender)

	delta := int64(raw)

	if delta >= 0 {
		delta := uint64(delta)

		if balance < delta {
			return errors.New("stake: balance < delta")
		}

		ctx.WriteAccountBalance(tx.Sender, balance-delta)
		ctx.WriteAccountStake(tx.Sender, stake+delta)
	} else {
		delta := uint64(-delta)

		if stake < delta {
			return errors.New("stake: stake < delta")
		}

		ctx.WriteAccountBalance(tx.Sender, stake-delta)
		ctx.WriteAccountBalance(tx.Sender, balance+delta)
	}

	return nil
}

func ProcessContractTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

	if len(tx.Payload) == 0 {
		return errors.New("contract: no code specified for contract to-be-spawned")
	}

	if _, exists := ctx.ReadAccountContractCode(tx.ID); exists {
		return errors.New("contract: there already exists a contract spawned with the specified code")
	}

	ctx.WriteAccountContractCode(tx.ID, tx.Payload)
	return nil
}
