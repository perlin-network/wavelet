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
	"github.com/chzyer/readline"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
)

type CLI struct {
	rl     *readline.Instance
	client *skademlia.Client
	ledger *wavelet.Ledger
	logger zerolog.Logger
	keys   *skademlia.Keypair
	tree   string
}

func NewCLI(client *skademlia.Client, ledger *wavelet.Ledger, keys *skademlia.Keypair) (*CLI, error) {
	completer := readline.NewPrefixCompleter(
		readline.PcItem("l"), readline.PcItem("status"),
		readline.PcItem("p"), readline.PcItem("pay"),
		readline.PcItem("c"), readline.PcItem("call"),
		readline.PcItem("f"), readline.PcItem("find"),
		readline.PcItem("s"), readline.PcItem("spawn"),
		readline.PcItem("ps"), readline.PcItem("place-stake"),
		readline.PcItem("ws"), readline.PcItem("withdraw-stake"),
		readline.PcItem("wr"), readline.PcItem("withdraw-reward"),
		readline.PcItem("help"),
	)

	rl, err := readline.NewEx(
		&readline.Config{
			Prompt:            "\033[31m»»»\033[0m ",
			AutoComplete:      completer,
			HistoryFile:       "/tmp/readline.tmp",
			InterruptPrompt:   "^C",
			EOFPrompt:         "exit",
			HistorySearchFold: true,
		},
	)
	if err != nil {
		return nil, err
	}

	log.SetWriter(log.LoggerWavelet, log.NewConsoleWriter(rl.Stderr(), log.FilterFor(log.ModuleNode, log.ModuleNetwork, log.ModuleSync, log.ModuleConsensus, log.ModuleContract)))

	return &CLI{
		rl:     rl,
		client: client,
		ledger: ledger,
		logger: log.Node(),
		tree:   completer.Tree("    "),
		keys:   keys,
	}, nil
}

func toCMD(in string, n int) []string {
	in = strings.TrimSpace(in[n:])
	if in == "" {
		return []string{}
	}

	return strings.Split(in, " ")
}

func (cli *CLI) Start() {
	defer func() {
		_ = cli.rl.Close()
	}()

	for {
		line, err := cli.rl.Readline()
		switch err {
		case readline.ErrInterrupt:
			if len(line) == 0 {
				return
			} else {
				continue
			}
		case io.EOF:
			return
		}

		switch {
		case line == "l" || line == "status":
			cli.status()
		case strings.HasPrefix(line, "p "):
			cli.pay(toCMD(line, 2), nil)
		case strings.HasPrefix(line, "pay "):
			cli.pay(toCMD(line, 4), nil)
		case strings.HasPrefix(line, "call "):
			cli.call(toCMD(line, 5))
		case strings.HasPrefix(line, "f "):
			cli.find(toCMD(line, 2))
		case strings.HasPrefix(line, "find "):
			cli.find(toCMD(line, 5))
		case strings.HasPrefix(line, "s "):
			cli.spawn(toCMD(line, 2))
		case strings.HasPrefix(line, "spawn "):
			cli.spawn(toCMD(line, 5))
		case strings.HasPrefix(line, "ps "):
			cli.placeStake(toCMD(line, 3))
		case strings.HasPrefix(line, "place-stake "):
			cli.placeStake(toCMD(line, 12))
		case strings.HasPrefix(line, "ws "):
			cli.withdrawStake(toCMD(line, 3))
		case strings.HasPrefix(line, "withdraw-stake "):
			cli.withdrawStake(toCMD(line, 15))
		case strings.HasPrefix(line, "withdraw-reward "):
			cli.withdrawReward(toCMD(line, 16))
		case line == "":
			fallthrough
		case line == "help":
			cli.usage()
		default:
			fmt.Printf("unrecognised command :'%s'\n", line)
		}
	}
}

func (cli *CLI) usage() {
	_, _ = io.WriteString(cli.rl.Stderr(), "commands:\n")
	_, _ = io.WriteString(cli.rl.Stderr(), cli.tree)
}

func (cli *CLI) status() {
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
	nonce, _ := wavelet.ReadAccountNonce(snapshot, publicKey)

	round := cli.ledger.Rounds().Latest()
	rootDepth := cli.ledger.Graph().RootDepth()

	peers := cli.client.ClosestPeerIDs()
	peerIDs := make([]string, 0, len(peers))

	for _, id := range peers {
		peerIDs = append(peerIDs, id.String())
	}

	cli.logger.Info().
		Uint8("difficulty", round.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
		Uint64("round", round.Index).
		Hex("root_id", round.End.ID[:]).
		Uint64("height", cli.ledger.Graph().Height()).
		Str("id", hex.EncodeToString(publicKey[:])).
		Uint64("balance", balance).
		Uint64("stake", stake).
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

func (cli *CLI) pay(cmd []string, additional []byte) {
	if len(cmd) < 2 || len(cmd) > 3 {
		fmt.Println("pay <recipient> <amount> [gas-limit]")
		return
	}

	recipient, err := hex.DecodeString(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).Msg("The recipient you specified is invalid.")
		return
	}

	if len(recipient) != wavelet.SizeAccountID {
		cli.logger.Error().Int("length", len(recipient)).Msg("You have specified an invalid account ID to find.")
		return
	}

	amount, err := strconv.Atoi(cmd[1])
	if err != nil {
		cli.logger.Error().Err(err).Msg("Failed to convert payment amount to an uint64.")
		return
	}

	payload := bytes.NewBuffer(nil)
	payload.Write(recipient[:])

	var intBuf [8]byte
	binary.LittleEndian.PutUint64(intBuf[:], uint64(amount))
	payload.Write(intBuf[:])

	gasLimit := 0
	if len(cmd) == 3 {
		gasLimit, err = strconv.Atoi(cmd[2])
		if err != nil {
			cli.logger.Error().
				Err(err).
				Str("gas-limit", cmd[2]).
				Msg("Failed to convert gas-limit.")
			return
		}
	}

	binary.LittleEndian.PutUint64(intBuf[:], uint64(gasLimit))
	payload.Write(intBuf[:])

	if additional != nil {
		payload.Write(additional)
	}

	tx, err := cli.sendTransaction(wavelet.NewTransaction(cli.keys, sys.TagTransfer, payload.Bytes()))
	if err != nil {
		return
	}

	cli.logger.Info().Msgf("Success! Your payment transaction ID: %x", tx.ID)
}

func (cli *CLI) call(cmd []string) {
	if len(cmd) < 4 {
		fmt.Println("call <smart-contract-address> <amount> <gas-limit> <function> [function parameters]")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(cmd[3])))
	payload.Write(intBuf[:4])
	payload.WriteString(cmd[3])

	params := bytes.NewBuffer(nil)

	for i := 4; i < len(cmd); i++ {
		arg := cmd[i]

		switch arg[0] {
		case 'S':
			binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(arg[1:])))
			params.Write(intBuf[:4])
			params.WriteString(arg[1:])
		case 'B':
			binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(arg[1:])))
			params.Write(intBuf[:4])
			params.Write([]byte(arg[1:]))
		case '1', '2', '4', '8':
			var val uint64
			_, err := fmt.Sscanf(arg[1:], "%d", &val)
			if err != nil {
				cli.logger.Error().Err(err).Msgf("Got an error parsing integer: %+v", arg[1:])
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
				cli.logger.Error().Err(err).Msgf("Cannot decode hex: %s", arg[1:])
				return
			}

			params.Write(buf)
		default:
			cli.logger.Error().Msgf("Invalid argument specified: %s", arg)
			return
		}

		buf := params.Bytes()

		binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(buf)))
		payload.Write(intBuf[:4])
		payload.Write(buf)
	}

	cli.pay(cmd[:3], payload.Bytes())
}

func (cli *CLI) find(cmd []string) {
	if len(cmd) != 1 {
		fmt.Println("find <tx-id | wallet-address>")
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
		cli.logger.Error().Int("length", len(buf)).Msg("You have specified an invalid transaction/account ID to find.")
		return
	}

	var accountID wavelet.AccountID
	copy(accountID[:], buf)

	balance, _ := wavelet.ReadAccountBalance(snapshot, accountID)
	stake, _ := wavelet.ReadAccountStake(snapshot, accountID)
	nonce, _ := wavelet.ReadAccountNonce(snapshot, accountID)

	_, isContract := wavelet.ReadAccountContractCode(snapshot, accountID)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, accountID)

	if balance > 0 || stake > 0 || nonce > 0 || isContract || numPages > 0 {
		cli.logger.Info().
			Uint64("balance", balance).
			Uint64("stake", stake).
			Uint64("nonce", nonce).
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
			Uint8("tag", tx.Tag).
			Uint64("depth", tx.Depth).
			Hex("seed", tx.Seed[:]).
			Uint8("seed_zero_prefix_len", tx.SeedLen).
			Msgf("Transaction: %s", cmd[0])

		return
	}
}

func (cli *CLI) spawn(cmd []string) {
	if len(cmd) != 1 {
		fmt.Println("spawn <path-to-smart-contract>")
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

	binary.LittleEndian.PutUint64(buf[:], 0) // Payload size.
	w.Write(buf[:])

	w.Write(code) // Smart contract code.

	tx := wavelet.NewTransaction(cli.keys, sys.TagContract, w.Bytes())

	tx, err = cli.sendTransaction(tx)
	if err != nil {
		return
	}

	cli.logger.Info().Msgf("Success! Your smart contracts ID: %x", tx.ID)
}

func (cli *CLI) placeStake(cmd []string) {
	if len(cmd) != 1 {
		fmt.Println("place-stake <amount>")
		return
	}

	amount, err := strconv.Atoi(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).Msg("Failed to convert staking amount to a uint64.")
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

	cli.logger.Info().
		Msgf("Success! Your stake placement transaction ID: %x", tx.ID)
}

func (cli *CLI) withdrawStake(cmd []string) {
	if len(cmd) != 1 {
		fmt.Println("withdraw-stake <amount>")
		return
	}

	amount, err := strconv.ParseUint(cmd[0], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).Msg("Failed to convert withdraw amount to an uint64.")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.WithdrawStake)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])

	tx, err := cli.sendTransaction(wavelet.NewTransaction(cli.keys, sys.TagStake, payload.Bytes()))
	if err != nil {
		return
	}

	cli.logger.Info().
		Msgf("Success! Your stake withdrawal transaction ID: %x", tx.ID)
}

func (cli *CLI) withdrawReward(cmd []string) {
	if len(cmd) != 1 {
		fmt.Println("withdraw-reward <amount>")
		return
	}

	amount, err := strconv.ParseUint(cmd[0], 10, 64)
	if err != nil {
		cli.logger.Error().Err(err).Msg("Failed to convert withdraw amount to an uint64.")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.WriteByte(sys.WithdrawReward)
	binary.LittleEndian.PutUint64(intBuf[:8], uint64(amount))
	payload.Write(intBuf[:8])

	tx, err := cli.sendTransaction(wavelet.NewTransaction(cli.keys, sys.TagStake, payload.Bytes()))
	if err != nil {
		return
	}

	cli.logger.Info().
		Msgf("Success! Your reward withdrawal transaction ID: %x", tx.ID)
}

func (cli *CLI) sendTransaction(tx wavelet.Transaction) (wavelet.Transaction, error) {
	tx = wavelet.AttachSenderToTransaction(cli.keys, tx, cli.ledger.Graph().FindEligibleParents()...)

	if err := cli.ledger.AddTransaction(tx); err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
		cli.logger.
			Err(err).
			Hex("tx_id", tx.ID[:]).
			Msg("Failed to create your transaction.")

		return tx, err
	}

	return tx, nil
}
