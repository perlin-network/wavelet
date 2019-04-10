package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"io"
)

func ProcessNopTransaction(_ *TransactionContext) error {
	return nil
}

func ProcessTransferTransaction(ctx *TransactionContext) error {
	tx := ctx.Transaction()

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
	var err error

	if r.Len() > 0 {
		if _, err := io.ReadFull(r, buf[:4]); err != nil {
			return errors.Wrap(err, "transfer: failed to decode smart contract function name to invoke")
		}

		funcName := make([]byte, binary.LittleEndian.Uint32(buf[:4]))

		if _, err := io.ReadFull(r, funcName); err != nil {
			return errors.Wrap(err, "transfer: failed to decode smart contract function name to invoke")
		}

		if _, err := io.ReadFull(r, buf[:4]); err != nil {
			return errors.Wrap(err, "transfer: failed to decode smart contract function invocation parameters")
		}

		funcParams := make([]byte, binary.LittleEndian.Uint32(buf[:4]))

		if _, err := io.ReadFull(r, funcParams); err != nil {
			return errors.Wrap(err, "transfer: failed to decode smart contract function invocation parameters")
		}

		_, gas, err = executor.Run(amount, 50000000, string(funcName), funcParams...)
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

	r := bytes.NewReader(tx.Payload)

	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return errors.Wrap(err, "stake: failed to decode amount of stake to place/withdraw")
	}

	delta := int64(binary.LittleEndian.Uint64(buf[:]))

	balance, _ := ctx.ReadAccountBalance(tx.Creator)
	stake, _ := ctx.ReadAccountStake(tx.Creator)

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
