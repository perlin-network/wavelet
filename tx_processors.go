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

	creatorBalance, _ := ctx.ReadAccountBalance(tx.Creator)

	if creatorBalance < amount {
		return errors.Errorf("transfer: not enough balance, wanting %d PERLs", amount)
	}

	ctx.WriteAccountBalance(tx.Creator, creatorBalance-amount)

	recipientBalance, _ := ctx.ReadAccountBalance(recipient)
	ctx.WriteAccountBalance(recipient, recipientBalance+amount)

	if _, isContract := ctx.ReadAccountContractCode(recipient); !isContract {
		return nil
	}

	executor := NewContractExecutor(recipient, ctx).WithGasTable(sys.GasTable).EnableLogging()

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

	if creatorBalance < amount {
		return errors.Errorf("transfer: transaction creator tried to send %d PERLs, but only has %d PERLs", amount, creatorBalance)
	}

	ctx.WriteAccountBalance(tx.Creator, creatorBalance-amount-gas)

	logger := log.Contract(recipient, "gas")
	logger.Info().
		Uint64("gas", gas).
		Hex("creator_id", tx.Creator[:]).
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

	balance, _ := ctx.ReadAccountBalance(tx.Creator)
	stake, _ := ctx.ReadAccountStake(tx.Creator)

	delta := int64(raw)

	if delta >= 0 {
		delta := uint64(delta)

		if balance < delta {
			return errors.New("stake: balance < delta")
		}

		ctx.WriteAccountBalance(tx.Creator, balance-delta)
		ctx.WriteAccountStake(tx.Creator, stake+delta)
	} else {
		delta := uint64(-delta)

		if stake < delta {
			return errors.New("stake: stake < delta")
		}

		ctx.WriteAccountBalance(tx.Creator, stake-delta)
		ctx.WriteAccountBalance(tx.Creator, balance+delta)
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

	executor := NewContractExecutor(tx.ID, ctx).WithGasTable(sys.GasTable)

	vm, err := executor.Init(tx.Payload, 50000000)
	if err != nil {
		return errors.New("contract: code for contract is not valid WebAssembly code")
	}

	ctx.WriteAccountContractCode(tx.ID, tx.Payload)
	executor.SaveMemorySnapshot(tx.ID, vm.Memory)

	return nil
}
