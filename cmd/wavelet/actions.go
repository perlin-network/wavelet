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

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"strconv"

	wasm "github.com/perlin-network/life/wasm-validation"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func (cli *CLI) status(ctx *cli.Context) {
	l, err := cli.LedgerStatus("", "", 0, 0)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to get the ledger status")
		return
	}

	a, err := cli.GetAccount(l.PublicKey)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to get the account status")
		return
	}

	var peers = make([]string, 0, len(l.Peers))
	for _, p := range l.Peers {
		peers = append(peers, fmt.Sprintf("%s[%x]", p.Address, p.PublicKey))
	}

	cli.logger.Info().
		Uint64("difficulty", l.Round.Difficulty).
		Uint64("round", l.Round.Index).
		Hex("root_id", l.Round.EndID[:]).
		Uint64("height", l.Graph.Height).
		Hex("id", l.PublicKey[:]).
		Uint64("balance", a.Balance).
		Uint64("stake", a.Stake).
		Uint64("reward", a.Reward).
		Uint64("nonce", a.Nonce).
		Strs("peers", peers).
		Uint64("num_tx", l.Graph.Tx).
		Uint64("num_missing_tx", l.Graph.MissingTx).
		Uint64("num_tx_in_store", l.Graph.TxInStore).
		Int("num_accounts_in_store", l.NumAccounts).
		Str("preferred_id", l.PreferredID).
		Int("preferred_votes", l.PreferredVotes).
		Msg("Here is the current status of your node.")
}

func (cli *CLI) pay(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 2 {
		cli.logger.Error().
			Msg("Invalid usage: pay <recipient> <amount>")
		return
	}

	recipient, ok := cli.recipient(cmd[0])
	if !ok {
		return
	}

	amount, ok := cli.amount(cmd[1])
	if !ok {
		return
	}

	tx, err := cli.Pay(recipient, amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to pay to recipient.")
		return
	}

	cli.logger.Info().
		Msgf("Success! Your payment transaction ID: %x", tx.ID)
}

func (cli *CLI) call(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 4 {
		cli.logger.Error().
			Msg("Invalid usage: call <smart-contract-address> <amount> <gas-limit> <function> [function parameters]")
		return
	}

	recipient, ok := cli.recipient(cmd[0])
	if !ok {
		return
	}

	amount, ok := cli.amount(cmd[1])
	if !ok {
		return
	}

	gasLimit, ok := cli.amount(cmd[2])
	if !ok {
		return
	}

	fn := wctl.FunctionCall{
		Name:     cmd[3],
		Amount:   amount,
		GasLimit: gasLimit,
	}

	var intBuf [8]byte
	for i := 4; i < len(cmd); i++ {
		arg := cmd[i]

		switch arg[0] {
		case 'S':
			fn.AddParams(wctl.EncodeString(arg[1:]))
		case 'B':
			fn.AddParams(wctl.EncodeBytes([]byte(arg[1:])))
		case '1', '2', '4', '8':
			var val uint64
			if _, err := fmt.Sscanf(arg[1:], "%d", &val); err != nil {
				cli.logger.Error().Err(err).
					Msgf("Got an error parsing integer: %+v", arg[1:])
				return
			}

			switch arg[0] {
			case '1':
				fn.AddParams(wctl.EncodeByte(byte(val)))
			case '2':
				binary.LittleEndian.PutUint16(intBuf[:2], uint16(val))
				fn.AddParams(wctl.EncodeBytes(intBuf[:2]))
			case '4':
				binary.LittleEndian.PutUint32(intBuf[:4], uint32(val))
				fn.AddParams(wctl.EncodeBytes(intBuf[:4]))
			case '8':
				binary.LittleEndian.PutUint64(intBuf[:8], uint64(val))
				fn.AddParams(wctl.EncodeBytes(intBuf[:8]))
			}
		case 'H':
			buf, err := wctl.DecodeHex(arg[1:])
			if err != nil {
				cli.logger.Error().Err(err).
					Msgf("Cannot decode hex: %s", arg[1:])
				return
			}

			fn.AddParams(buf)
		default:
			cli.logger.Error().
				Str("prefix", string(arg[0])).
				Msgf("Invalid argument prefix specified")
			return
		}
	}

	tx, err := cli.Call(recipient, fn)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to call function.")
		return
	}

	cli.logger.Info().
		Str("recipient", cmd[0]).
		Str("tx_id", tx.ID).
		Msgf("Smart contract function called.", tx.ID)
}

func (cli *CLI) find(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: find <tx-id | wallet-address>")
		return
	}

	snapshot := cli.ledger.Snapshot()

	address := cmd[0]

	buf, err := hex.DecodeString(address)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Cannot decode address")
		return
	}

	if len(buf) != wavelet.SizeTransactionID && len(buf) != wavelet.SizeAccountID {
		cli.logger.Error().Int("length", len(buf)).
			Msg("You have specified an invalid transaction/account ID to find.")
		return
	}

	var accountID wavelet.AccountID
	copy(accountID[:], buf)

	balance, _ := wavelet.ReadAccountBalance(snapshot, accountID)
	gasBalance, _ := wavelet.ReadAccountContractGasBalance(snapshot, accountID)
	stake, _ := wavelet.ReadAccountStake(snapshot, accountID)
	nonce, _ := wavelet.ReadAccountNonce(snapshot, accountID)
	reward, _ := wavelet.ReadAccountReward(snapshot, accountID)

	_, isContract := wavelet.ReadAccountContractCode(snapshot, accountID)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, accountID)

	if balance > 0 || stake > 0 || nonce > 0 || isContract || numPages > 0 {
		cli.logger.Info().
			Uint64("balance", balance).
			Uint64("gas_balance", gasBalance).
			Uint64("stake", stake).
			Uint64("nonce", nonce).
			Uint64("reward", reward).
			Bool("is_contract", isContract).
			Uint64("num_pages", numPages).
			Msgf("Account: %s", cmd[0])

		return
	}

	var txID wavelet.TransactionID
	copy(txID[:], buf)

	tx := cli.ledger.Graph().FindTransaction(txID)

	if tx != nil {
		var parents []string
		for _, parentID := range tx.ParentIDs {
			parents = append(parents, hex.EncodeToString(parentID[:]))
		}

		cli.logger.Info().
			Strs("parents", parents).
			Hex("sender", tx.Sender[:]).
			Hex("creator", tx.Creator[:]).
			Uint64("nonce", tx.Nonce).
			Uint8("tag", byte(tx.Tag)).
			Uint64("depth", tx.Depth).
			Hex("seed", tx.Seed[:]).
			Uint8("seed_zero_prefix_len", tx.SeedLen).
			Msgf("Transaction: %s", cmd[0])

		return
	}
}

func (cli *CLI) spawn(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: spawn <path-to-smart-contract>")
		return
	}

	code, err := ioutil.ReadFile(cmd[0])
	if err != nil {
		cli.logger.Error().
			Err(err).
			Str("path", cmd[0]).
			Msg("Failed to find/load the smart contract code from the given path.")
		return
	}

	if err := wasm.GetValidator().ValidateWasm(code); err != nil {
		cli.logger.Error().
			Err(err).
			Str("path", cmd[0]).
			Msg("Invalid wasm")
		return
	}

	payload := wavelet.Contract{
		GasLimit: 100000000,
		Code:     code,
	}

	tx, err := cli.sendTransaction(wavelet.NewTransaction(cli.keys, sys.TagContract, payload.Marshal()))
	if err != nil {
		return
	}

	cli.logger.Info().Msgf("Success! Your smart contracts ID: %x", tx.ID)
}

func (cli *CLI) depositGas(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 2 {
		cli.logger.Error().
			Msg("Invalid usage: deposit-gas <recipient> <amount>")
		return
	}

	// Get the recipient ID
	recipient, err := hex.DecodeString(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("The recipient you specified is invalid.")
		return
	}

	// Check if the ID is actually invalid by length
	if len(recipient) != wavelet.SizeAccountID {
		cli.logger.Error().Int("length", len(recipient)).
			Msg("You have specified an invalid account ID to find.")
		return
	}

	// Parse the gas amount
	amount, err := strconv.ParseUint(cmd[1], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert payment amount to an uint64.")
		return
	}

	// Make a new payload, copy the recipient over and assign the amount
	var payload wavelet.Transfer
	copy(payload.Recipient[:], recipient)
	payload.GasDeposit = amount

	// Get snapshot
	snapshot := cli.ledger.Snapshot()

	// Get balance and check if recipient is a smart contract
	balance, _ := wavelet.ReadAccountBalance(snapshot, cli.keys.PublicKey())
	_, codeAvailable := wavelet.ReadAccountContractCode(snapshot, payload.Recipient)

	// Check balance
	if balance < amount+sys.TransactionFeeAmount {
		cli.logger.Error().
			Uint64("your_balance", balance).
			Uint64("amount_to_send", amount).
			Msg("You do not have enough PERLs to deposit into the smart contract.")
		return
	}

	// The recipient is not a smart contract
	if !codeAvailable {
		cli.logger.Error().Hex("recipient_id", recipient).
			Msg("The recipient you specified is not a smart contract.")
		return
	}

	tx, err := cli.sendTransaction(
		wavelet.NewTransaction(cli.keys, sys.TagTransfer, payload.Marshal()),
	)

	if err != nil {
		return
	}

	cli.logger.Info().
		Msgf("Success! Your gas deposit transaction ID: %x", tx.ID)
}

func (cli *CLI) placeStake(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: place-stake <amount>")
		return
	}

	amount, err := strconv.ParseUint(cmd[0], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert staking amount to a uint64.")
		return
	}

	payload := wavelet.Stake{
		Opcode: sys.PlaceStake,
		Amount: amount,
	}

	tx, err := cli.sendTransaction(wavelet.NewTransaction(
		cli.keys, sys.TagStake, payload.Marshal(),
	))

	if err != nil {
		return
	}

	cli.logger.Info().
		Msgf("Success! Your stake placement transaction ID: %x", tx.ID)
}

func (cli *CLI) withdrawStake(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: withdraw-stake <amount>")
		return
	}

	amount, err := strconv.ParseUint(cmd[0], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert withdraw amount to an uint64.")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.WithdrawStake)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])

	tx, err := cli.sendTransaction(wavelet.NewTransaction(
		cli.keys, sys.TagStake, payload.Bytes(),
	))

	if err != nil {
		return
	}

	txID := hex.EncodeToString(tx.ID[:])

	cli.logger.Info().
		Msg("Success! Your stake withdrawal transaction ID: " + txID)
}

func (cli *CLI) withdrawReward(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: withdraw-reward <amount>")
		return
	}

	amount, err := strconv.ParseUint(cmd[0], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert withdraw amount to an uint64.")
		return
	}

	payload := wavelet.Stake{
		Opcode: sys.WithdrawReward,
		Amount: amount,
	}

	tx, err := cli.sendTransaction(wavelet.NewTransaction(
		cli.keys, sys.TagStake, payload.Marshal(),
	))

	if err != nil {
		return
	}

	cli.logger.Info().
		Msgf("Success! Your reward withdrawal transaction ID: %x", tx.ID)
}

func (cli *CLI) sendTransaction(tx wavelet.Transaction) (wavelet.Transaction, error) {
	tx = wavelet.AttachSenderToTransaction(
		cli.keys, tx, cli.ledger.Graph().FindEligibleParents()...,
	)

	if err := cli.ledger.AddTransaction(tx); err != nil {
		if errors.Cause(err) != wavelet.ErrMissingParents {
			cli.logger.
				Err(err).
				Hex("tx_id", tx.ID[:]).
				Msg("Failed to create your transaction.")

			return tx, err
		}
	}

	return tx, nil
}
