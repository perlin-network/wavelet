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
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/urfave/cli"
)

func (cli *CLI) status(ctx *cli.Context) {
	l, err := cli.client.LedgerStatus("", "", 0, 0)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to get the ledger status")
		return
	}

	a, _ := cli.client.GetAccount(l.PublicKey)
	if a == nil {
		a = &wctl.Account{}
	}

	preferredID := "N/A"

	if l.Preferred != nil {
		preferredID = hex.EncodeToString(l.Preferred.ID[:])
	}

	var peers = make([]string, 0, len(l.Peers))
	for _, p := range l.Peers {
		peers = append(peers, fmt.Sprintf("%s[%x]", p.Address, p.PublicKey))
	}

	cli.logger.Info().
		Uint64("block_height", l.Block.Index).
		Hex("block_id", l.Block.ID[:]).
		Hex("user_id", l.PublicKey[:]).
		Uint64("balance", a.Balance).
		Uint64("stake", a.Stake).
		Uint64("reward", a.Reward).
		Uint64("nonce", a.Nonce).
		Strs("peers", peers).
		Uint64("num_tx", l.NumTx).
		Uint64("num_missing_tx", l.NumMissingTx).
		Uint64("num_tx_in_store", l.NumTxInStore).
		Uint64("num_accounts_in_store", l.AccountsLen).
		Uint64("client_nonce", cli.client.Nonce.Load()).
		Uint64("client_block", cli.client.Block.Load()).
		Str("sync_status", l.SyncStatus).
		Str("preferred_block_id", preferredID).
		Int("preferred_votes", l.PreferredVotes).
		Msg("Here is the current status of your node.")
}

func (cli *CLI) pay(ctx *cli.Context) {
	cmd := ctx.Args()

	if len(cmd) < 2 {
		cli.logger.Error().
			Msg("Invalid usage: pay <recipient> <amount>")
		return
	}

	recipient, ok := cli.parseRecipient(cmd[0])
	if !ok {
		return
	}

	amount, ok := cli.parseAmount(cmd[1])
	if !ok {
		return
	}

	tx, err := cli.client.Pay(recipient, amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to pay to recipient.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Paid to recipient.")
}

func (cli *CLI) call(ctx *cli.Context) {
	cmd := ctx.Args()

	if len(cmd) < 4 {
		cli.logger.Error().
			Msg("Invalid usage: call <smart-contract-address> <amount> <gas-limit> <function> [function parameters]")
		return
	}

	recipient, ok := cli.parseRecipient(cmd[0])
	if !ok {
		return
	}

	amount, ok := cli.parseAmount(cmd[1])
	if !ok {
		return
	}

	gasLimit, ok := cli.parseAmount(cmd[2])
	if !ok {
		return
	}

	fn := wctl.FunctionCall{
		Name:     cmd[3],
		Amount:   amount,
		GasLimit: gasLimit,
	}

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
				fn.AddParams(wctl.EncodeUint16(uint16(val)))
			case '4':
				fn.AddParams(wctl.EncodeUint32(uint32(val)))
			case '8':
				fn.AddParams(wctl.EncodeUint64(val))
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

	tx, err := cli.client.Call(recipient, fn)
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
	cmd := ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: find <tx-id | wallet-address>")
		return
	}

	address, ok := cli.parseRecipient(cmd[0])
	if !ok {
		return
	}

	account, tx, err := cli.client.Find(address)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Cannot find address")
		return
	}

	switch {
	case account != nil:
		cli.logger.Info().
			Hex("public_key", account.PublicKey[:]).
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
			Hex("sender", tx.Sender[:]).
			Uint64("nonce", tx.Nonce).
			Uint8("tag", tx.Tag).
			Msgf("Transaction: %s", cmd[0])
	}
}

func (cli *CLI) spawn(ctx *cli.Context) {
	cmd := ctx.Args()

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

	tx, err := cli.client.Spawn(code, 100000000)
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
	cmd := ctx.Args()

	if len(cmd) < 2 {
		cli.logger.Error().
			Msg("Invalid usage: deposit-gas <recipient> <amount>")
		return
	}

	recipient, ok := cli.parseRecipient(cmd[0])
	if !ok {
		return
	}

	amount, ok := cli.parseAmount(cmd[1])
	if !ok {
		return
	}

	tx, err := cli.client.DepositGas(recipient, amount)
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
	cmd := ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: place-stake <amount>")
		return
	}

	amount, ok := cli.parseAmount(cmd[0])
	if !ok {
		return
	}

	tx, err := cli.client.PlaceStake(amount)
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
	cmd := ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: withdraw-stake <amount>")
		return
	}

	amount, ok := cli.parseAmount(cmd[0])
	if !ok {
		return
	}

	tx, err := cli.client.WithdrawStake(amount)
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
	cmd := ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: withdraw-reward <amount>")
		return
	}

	amount, ok := cli.parseAmount(cmd[0])
	if !ok {
		return
	}

	tx, err := cli.client.WithdrawReward(amount)
	if err != nil {
		cli.logger.Err(err).
			Msg("Failed to withdraw reward.")
		return
	}

	cli.logger.Info().
		Hex("tx_id", tx.ID[:]).
		Msgf("Reward withdrew.")
}

func (cli *CLI) connect(ctx *cli.Context) {
	cmd := ctx.Args()

	if len(cmd) != 1 {
		cli.logger.Error().Msg("Invalid usage: connect <address:port>")
		return
	}

	m, err := cli.client.Connect(cmd[0])
	if err != nil {
		cli.logger.Error().
			Err(err).
			Msg("Failed to connect to address.")
		return
	}

	cli.logger.Info().Msg(m.Message)
}

func (cli *CLI) disconnect(ctx *cli.Context) {
	cmd := ctx.Args()

	if len(cmd) != 1 {
		cli.logger.Error().Msg("Invalid usage: disconnect <address:port>")
		return
	}

	m, err := cli.client.Disconnect(cmd[0])
	if err != nil {
		cli.logger.Error().
			Err(err).
			Msg("Failed to disconnect to address.")
		return
	}

	cli.logger.Info().Msg(m.Message)
}

func (cli *CLI) restart(ctx *cli.Context) {
	cmd := ctx.Args()
	if len(cmd) != 0 {
		cli.logger.Error().Msg("Invalid usage: restart [--hard]")
		return
	}

	m, err := cli.client.Restart(ctx.Bool("hard"))
	if err != nil {
		cli.logger.Error().
			Err(err).
			Msg("Failed to restart node.")
		return
	}

	cli.logger.Info().Msg(m.Message)
}

func (cli *CLI) updateParameters(ctx *cli.Context) {
	conf.Update(
		conf.WithSnowballK(ctx.Int("snowball.k")),
		conf.WithSnowballBeta(ctx.Int("snowball.beta")),
		conf.WithSyncVoteThreshold(ctx.Float64("vote.sync.threshold")),
		conf.WithFinalizationVoteThreshold(ctx.Float64("vote.finalization.threshold")),
		conf.WithStakeMajorityWeight(ctx.Float64("vote.finalization.stake.weight")),
		conf.WithTransactionsNumMajorityWeight(ctx.Float64("vote.finalization.transactions.weight")),
		conf.WithQueryTimeout(ctx.Duration("query.timeout")),
		conf.WithGossipTimeout(ctx.Duration("gossip.timeout")),
		conf.WithDownloadTxTimeout(ctx.Duration("download.tx.timeout")),
		conf.WithCheckOutOfSyncTimeout(ctx.Duration("check.out.of.sync.timeout")),
		conf.WithSyncChunkSize(ctx.Int("sync.chunk.size")),
		conf.WithSyncIfBlockIndicesDifferBy(ctx.Uint64("sync.if.rounds.differ.by")),
		conf.WithPruningLimit(uint8(ctx.Uint64("pruning.limit"))),
		conf.WithSecret(ctx.String("api.secret")),
		conf.WithBloomFilterK(ctx.Uint("bloom.filter.k")),
		conf.WithBloomFilterM(ctx.Uint("bloom.filter.m")),
		conf.WithTXSyncChunkSize(ctx.Uint64("tx.sync.chunk.size")),
		conf.WithTXSyncLimit(ctx.Uint64("tx.sync.limit")),
	)

	cli.logger.Info().Str("conf", conf.Stringify()).
		Msg("Current configuration values")
}
