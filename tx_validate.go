// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package wavelet

import (
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

func ValidateTransaction(snapshot *avl.Tree, tx Transaction) error {
	switch tx.Tag {
	case sys.TagTransfer:
		return validateTransferTransaction(snapshot, tx)
	case sys.TagStake:
		return validateStakeTransaction(snapshot, tx)
	case sys.TagContract:
		return validateContractTransaction(snapshot, tx)
	case sys.TagBatch:
		return validateBatchTransaction(snapshot, tx)
	}

	return nil
}

func validateTransferTransaction(snapshot *avl.Tree, tx Transaction) error {
	payload, err := ParseTransfer(tx.Payload)
	if err != nil {
		return errors.Wrap(err, "could not parse transfer payload")
	}

	_, codeAvailable := ReadAccountContractCode(snapshot, payload.Recipient)
	if !codeAvailable && (payload.GasLimit > 0 || len(payload.FuncName) > 0 || len(payload.FuncParams) > 0) {
		return errors.New(
			"transfer: transactions to non-contract accounts should not specify gas limit or function names or params",
		)
	}

	if bal, exist := ReadAccountBalance(snapshot, tx.Sender); !exist {
		return errors.New("sender does not exist")
	} else if bal < tx.Fee()+payload.Amount+payload.GasLimit+payload.GasDeposit {
		return errors.Errorf("sender current balance %d is not enough", bal)
	}

	return nil
}

func validateStakeTransaction(snapshot *avl.Tree, tx Transaction) error {
	payload, err := ParseStake(tx.Payload)
	if err != nil {
		return err
	}

	balance, _ := ReadAccountBalance(snapshot, tx.Sender)
	stake, _ := ReadAccountStake(snapshot, tx.Sender)
	reward, _ := ReadAccountReward(snapshot, tx.Sender)

	switch payload.Opcode {
	case sys.PlaceStake:
		if balance < payload.Amount {
			return errors.Errorf(
				"stake: %x attempt to place a stake of %d PERLs, but only has %d PERLs",
				tx.Sender, payload.Amount, balance,
			)
		}
	case sys.WithdrawStake:
		if stake < payload.Amount {
			return errors.Errorf(
				"stake: %x attempt to withdraw a stake of %d PERLs, but only has staked %d PERLs",
				tx.Sender, payload.Amount, payload,
			)
		}
	case sys.WithdrawReward:
		if payload.Amount < sys.MinimumRewardWithdraw {
			return errors.Errorf(
				"stake: %x attempt to withdraw rewards amounting to %d PERLs, but system requires the minimum "+
					"amount to withdraw to be %d PERLs",
				tx.Sender, payload.Amount, sys.MinimumRewardWithdraw,
			)
		}

		if reward < payload.Amount {
			return errors.Errorf(
				"stake: %x attempt to withdraw rewards amounting to %d PERLs, but only has rewards amounting "+
					"to %d PERLs",
				tx.Sender, payload.Amount, reward,
			)
		}
	}

	return nil
}

func validateContractTransaction(snapshot *avl.Tree, tx Transaction) error {
	payload, err := ParseContract(tx.Payload)
	if err != nil {
		return err
	}

	if _, exists := ReadAccountContractCode(snapshot, tx.ID); exists {
		return errors.New("contract: already exists")
	}

	if bal, _ := ReadAccountBalance(snapshot, tx.Sender); bal < tx.Fee()+payload.GasDeposit+payload.GasLimit {
		return errors.Errorf("sender current balance %d is not enough", bal)
	}

	return nil
}

func validateBatchTransaction(snapshot *avl.Tree, tx Transaction) error {
	payload, err := ParseBatch(tx.Payload)
	if err != nil {
		return err
	}

	for i := uint8(0); i < payload.Size; i++ {
		entry := Transaction{
			ID:      tx.ID,
			Sender:  tx.Sender,
			Nonce:   tx.Nonce,
			Tag:     sys.Tag(payload.Tags[i]),
			Payload: payload.Payloads[i],
		}
		if err := ValidateTransaction(snapshot, entry); err != nil {
			return errors.Wrapf(err, "Error while processing %d/%d transaction in a batch.", i+1, payload.Size)
		}
	}

	return nil
}
