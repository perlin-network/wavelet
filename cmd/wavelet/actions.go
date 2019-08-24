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
	"encoding/binary"
	"fmt"
	"io/ioutil"

	"github.com/perlin-network/wavelet/wctl"
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
		Hex("tx_id", tx.ID[:]).
		Msgf("Smart contract function called.")
}

func (cli *CLI) find(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: find <tx-id | wallet-address>")
		return
	}

	address, ok := cli.recipient(cmd[0])
	if !ok {
		return
	}

	account, tx, err := cli.Find(address)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Cannot find address")
		return
	}

	switch {
	case account != nil:
		cli.logger.Info().
			Uint64("balance", account.Balance).
			Uint64("gas_balance", account.GasBalance).
			Uint64("stake", account.Stake).
			Uint64("nonce", account.Nonce).
			Uint64("reward", account.Reward).
			Bool("is_contract", account.IsContract).
			Uint64("num_pages", account.NumPages).
			Msgf("Account: %s", cmd[0])
	case tx != nil:
		cli.logger.Info().
			Strs("parents", wctl.StringIDs(tx.Parents)).
			Hex("sender", tx.Sender[:]).
			Hex("creator", tx.Creator[:]).
			Uint64("nonce", tx.Nonce).
			Uint8("tag", byte(tx.Tag)).
			Uint64("depth", tx.Depth).
			Hex("seed", tx.Seed[:]).
			Uint8("seed_zero_prefix_len", tx.SeedLen).
			Msgf("Transaction: %s", cmd[0])
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

	/*
		if err := wasm.GetValidator().ValidateWasm(code); err != nil {
			cli.logger.Error().
				Err(err).
				Str("path", cmd[0]).
				Msg("Invalid wasm")
			return
		}
	*/

	tx, err := cli.Spawn(code, 100000000)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to spawn smart contract.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Smart contract spawned.")
}

func (cli *CLI) depositGas(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 2 {
		cli.logger.Error().
			Msg("Invalid usage: deposit-gas <recipient> <amount>")
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

	tx, err := cli.DepositGas(recipient, amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to deposit gas.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Gas deposited.")
}

func (cli *CLI) placeStake(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: place-stake <amount>")
		return
	}

	amount, ok := cli.amount(cmd[0])
	if !ok {
		return
	}

	tx, err := cli.PlaceStake(amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to place stake.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Stake placed.")
}

func (cli *CLI) withdrawStake(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: withdraw-stake <amount>")
		return
	}

	amount, ok := cli.amount(cmd[0])
	if !ok {
		return
	}

	tx, err := cli.WithdrawStake(amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to withdraw stake.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Stake withdrew.")
}

func (cli *CLI) withdrawReward(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: withdraw-reward <amount>")
		return
	}

	amount, ok := cli.amount(cmd[0])
	if !ok {
		return
	}

	tx, err := cli.WithdrawReward(amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to withdraw reward.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Reward withdrew.")
}
