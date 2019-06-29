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

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func (cli *CLI) status(ctx *cli.Context) {
	preferredID := "N/A"

	if preferred := cli.ledger.Finalizer().Preferred(); preferred != nil {
		preferredID = hex.EncodeToString(preferred.ID[:])
	}

	count := cli.ledger.Finalizer().Progress()

	snapshot := cli.ledger.Snapshot()
	publicKey := cli.keys.PublicKey()

	accountsLen := wavelet.ReadAccountsLen(snapshot)

	balance, _ := wavelet.ReadAccountBalance(snapshot, publicKey)
	stake, _ := wavelet.ReadAccountStake(snapshot, publicKey)
	reward, _ := wavelet.ReadAccountReward(snapshot, publicKey)
	nonce, _ := wavelet.ReadAccountNonce(snapshot, publicKey)

	round := cli.ledger.Rounds().Latest()
	rootDepth := cli.ledger.Graph().RootDepth()

	peers := cli.client.ClosestPeerIDs()
	peerIDs := make([]string, 0, len(peers))

	for _, id := range peers {
		peerIDs = append(peerIDs, id.String())
	}

	// Add the IDs into the list of completions
	// TODO(diamond): Maybe make this into variables so you don't
	// convert it to strings twice
	cli.addCompletion(hex.EncodeToString(round.End.ID[:]))
	cli.addCompletion(hex.EncodeToString(publicKey[:]))
	for _, id := range peers {
		pub := id.PublicKey()
		cli.addCompletion(hex.EncodeToString(pub[:]))
	}

	cli.logger.Info().
		Uint8("difficulty", round.ExpectedDifficulty(
			sys.MinDifficulty,
			sys.DifficultyScaleFactor,
		)).
		Uint64("round", round.Index).
		Hex("root_id", round.End.ID[:]).
		Uint64("height", cli.ledger.Graph().Height()).
		Str("id", hex.EncodeToString(publicKey[:])).
		Uint64("balance", balance).
		Uint64("stake", stake).
		Uint64("reward", reward).
		Uint64("nonce", nonce).
		Strs("peers", peerIDs).
		Int("num_tx", cli.ledger.Graph().DepthLen(&rootDepth, nil)).
		Int("num_missing_tx", cli.ledger.Graph().MissingLen()).
		Int("num_tx_in_store", cli.ledger.Graph().Len()).
		Uint64("num_accounts_in_store", accountsLen).
		Str("preferred_id", preferredID).
		Int("preferred_votes", count).
		Msg("Here is the current status of your node.")
}

func (cli *CLI) pay(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 2 {
		cli.logger.Error().
			Msg("Invalid usage: pay <recipient> <amount>")
		return
	}

	recipient, err := hex.DecodeString(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("The recipient you specified is invalid.")
		return
	}

	if len(recipient) != wavelet.SizeAccountID {
		cli.logger.Error().Int("length", len(recipient)).
			Msg("You have specified an invalid account ID to find.")
		return
	}

	amount, err := strconv.ParseUint(cmd[1], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert payment amount to a uint64.")
		return
	}

	snapshot := cli.ledger.Snapshot()

	var recipientID wavelet.AccountID
	copy(recipientID[:], recipient)

	balance, _ := wavelet.ReadAccountBalance(snapshot, cli.keys.PublicKey())
	_, codeAvailable := wavelet.ReadAccountContractCode(snapshot, recipientID)

	if balance < amount {
		cli.logger.Error().
			Uint64("your_balance", balance).
			Uint64("amount_to_send", amount).
			Msg("You do not have enough PERLs to send.")
		return
	}

	payload := bytes.NewBuffer(nil)
	payload.Write(recipient[:])

	var intBuf [8]byte
	binary.LittleEndian.PutUint64(intBuf[:], amount)
	payload.Write(intBuf[:])

	if codeAvailable {
		// Set gas limit by default to the balance the user has.
		binary.LittleEndian.PutUint64(intBuf[:], balance)
		payload.Write(intBuf[:])

		defaultFuncName := "on_money_received"

		binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(defaultFuncName)))
		payload.Write(intBuf[:4])
		payload.WriteString(defaultFuncName)
	}

	tx, err := cli.sendTransaction(wavelet.NewTransaction(
		cli.keys, sys.TagTransfer, payload.Bytes(),
	))

	if err != nil {
		return
	}

	txID := hex.EncodeToString(tx.ID[:])

	// Add the ID into the completion list
	cli.addCompletion(txID)

	cli.logger.Info().Msg("Success! Your payment transaction ID: " + txID)
}

func (cli *CLI) call(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 4 {
		cli.logger.Error().
			Msg("Invalid usage: call <smart-contract-address> <amount> <gas-limit> <function> [function parameters]")
		return
	}

	recipient, err := hex.DecodeString(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("The smart contract address you specified is invalid.")
		return
	}

	if len(recipient) != wavelet.SizeAccountID {
		cli.logger.Error().Int("length", len(recipient)).
			Msg("You have specified an invalid account ID to find.")
		return
	}

	var recipientID wavelet.AccountID
	copy(recipientID[:], recipient)

	snapshot := cli.ledger.Snapshot()

	balance, _ := wavelet.ReadAccountBalance(snapshot, cli.keys.PublicKey())
	_, codeAvailable := wavelet.ReadAccountContractCode(snapshot, recipientID)

	if !codeAvailable {
		cli.logger.Error().
			Msg("The smart contract address you specified does not belong to a smart contract.")
		return
	}

	amount, err := strconv.ParseUint(cmd[1], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert payment amount to a uint64.")
		return
	}

	gasLimit, err := strconv.ParseUint(cmd[2], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert gas limit to a uint64.")
		return
	}

	if balance < amount+gasLimit {
		cli.logger.Error().
			Uint64("your_balance", balance).
			Uint64("cost", amount+gasLimit).
			Msg("You do not have enough PERLs to pay for the costs to invoke the smart contract function you wanted.")

		return
	}

	funcName := cmd[3]

	var intBuf [8]byte

	payload := bytes.NewBuffer(nil)

	// Recipient address 32 bytes.
	payload.Write(recipient[:])

	// Amount to send.
	binary.LittleEndian.PutUint64(intBuf[:8], amount)
	payload.Write(intBuf[:8])

	// Gas limit.
	// Set gas limit by default to the balance the user has.
	binary.LittleEndian.PutUint64(intBuf[:8], gasLimit)
	payload.Write(intBuf[:8])

	// Function name.
	binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(funcName)))
	payload.Write(intBuf[:4])
	payload.WriteString(funcName)

	params := bytes.NewBuffer(nil)

	for i := 4; i < len(cmd); i++ {
		arg := cmd[i]

		switch arg[0] {
		case 'S':
			params.WriteString(arg[1:])
			params.WriteByte(0)
		case 'B':
			binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(arg[1:])))
			params.Write(intBuf[:4])
			params.Write([]byte(arg[1:]))
		case '1', '2', '4', '8':
			var val uint64
			_, err := fmt.Sscanf(arg[1:], "%d", &val)
			if err != nil {
				cli.logger.Error().Err(err).
					Msgf("Got an error parsing integer: %+v", arg[1:])
			}

			switch arg[0] {
			case '1':
				params.WriteByte(byte(val))
			case '2':
				binary.LittleEndian.PutUint16(intBuf[:2], uint16(val))
				params.Write(intBuf[:2])
			case '4':
				binary.LittleEndian.PutUint32(intBuf[:4], uint32(val))
				params.Write(intBuf[:4])
			case '8':
				binary.LittleEndian.PutUint64(intBuf[:8], uint64(val))
				params.Write(intBuf[:8])
			}
		case 'H':
			buf, err := hex.DecodeString(arg[1:])

			if err != nil {
				cli.logger.Error().Err(err).
					Msgf("Cannot decode hex: %s", arg[1:])
				return
			}

			params.Write(buf)
		default:
			cli.logger.Error().
				Msgf("Invalid argument specified: %s", arg)
			return
		}
	}

	funcParams := params.Bytes()

	// Function payload.
	binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(funcParams)))
	payload.Write(intBuf[:4])
	payload.Write(funcParams)

	tx, err := cli.sendTransaction(wavelet.NewTransaction(
		cli.keys, sys.TagTransfer, payload.Bytes(),
	))

	if err != nil {
		return
	}

	txID := hex.EncodeToString(tx.ID[:])

	// Add the ID into the completion list
	cli.addCompletion(txID)

	cli.logger.Info().
		Msg("Success! Your smart contract invocation transaction ID: " + txID)
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
		cli.logger.Error().Err(err).Msg("Cannot decode address")
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
	stake, _ := wavelet.ReadAccountStake(snapshot, accountID)
	nonce, _ := wavelet.ReadAccountNonce(snapshot, accountID)
	reward, _ := wavelet.ReadAccountReward(snapshot, accountID)

	_, isContract := wavelet.ReadAccountContractCode(snapshot, accountID)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, accountID)

	if balance > 0 || stake > 0 || nonce > 0 || isContract || numPages > 0 {
		cli.logger.Info().
			Uint64("balance", balance).
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

		// Add the IDs into the completion list
		cli.addCompletion(cmd[0])
		cli.addCompletion(parents...)
		// TODO(diamond): maybe run this only once instead of twice in
		// logger.Info().Hex() as well?
		cli.addCompletion(hex.EncodeToString(tx.Sender[:]))
		cli.addCompletion(hex.EncodeToString(tx.Creator[:]))
		cli.addCompletion(hex.EncodeToString(tx.Seed[:]))

		cli.logger.Info().
			Strs("parents", parents).
			Hex("sender", tx.Sender[:]).
			Hex("creator", tx.Creator[:]).
			Uint64("nonce", tx.Nonce).
			Uint8("tag", tx.Tag).
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

	var buf [8]byte

	w := bytes.NewBuffer(nil)

	binary.LittleEndian.PutUint64(buf[:], 100000000) // Gas fee.
	w.Write(buf[:])

	binary.LittleEndian.PutUint32(buf[:4], 0) // Payload size.
	w.Write(buf[:4])

	w.Write(code) // Smart contract code.

	tx := wavelet.NewTransaction(cli.keys, sys.TagContract, w.Bytes())

	tx, err = cli.sendTransaction(tx)
	if err != nil {
		return
	}

	txID := hex.EncodeToString(tx.ID[:])

	// Add the ID into the completion list
	cli.addCompletion(txID)

	cli.logger.Info().
		Msg("Success! Your smart contract ID: " + txID)
}

func (cli *CLI) placeStake(ctx *cli.Context) {
	var cmd = ctx.Args()

	if len(cmd) < 1 {
		cli.logger.Error().
			Msg("Invalid usage: place-stake <amount>")
		return
	}

	amount, err := strconv.Atoi(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).
			Msg("Failed to convert staking amount to a uint64.")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.PlaceStake)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])

	tx, err := cli.sendTransaction(wavelet.NewTransaction(cli.keys, sys.TagStake, payload.Bytes()))
	if err != nil {
		return
	}

	txID := hex.EncodeToString(tx.ID[:])

	// Add the ID into the completion list
	cli.addCompletion(txID)

	cli.logger.Info().
		Msg("Success! Your stake placement transaction ID: " + txID)
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

	// Add the ID into the completion list
	cli.addCompletion(txID)

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

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.WithdrawReward)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])

	tx, err := cli.sendTransaction(wavelet.NewTransaction(
		cli.keys, sys.TagStake, payload.Bytes(),
	))

	if err != nil {
		return
	}

	txID := hex.EncodeToString(tx.ID[:])

	// Add the ID into the completion list
	cli.addCompletion(txID)

	cli.logger.Info().
		Msg("Success! Your reward withdrawal transaction ID: " + txID)
}

func (cli *CLI) sendTransaction(tx wavelet.Transaction) (wavelet.Transaction, error) {
	tx = wavelet.AttachSenderToTransaction(
		cli.keys, tx, cli.ledger.Graph().FindEligibleParents()...,
	)

	// Add the ID into the completion list
	cli.addCompletion(hex.EncodeToString(tx.ID[:]))

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
