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
	"strconv"
	"strings"
)

type CLI struct {
	rl *readline.Instance
	ledger *wavelet.Ledger
	logger zerolog.Logger
	keys *skademlia.Keypair
	tree string
}

func NewCLI(
	ledger *wavelet.Ledger, keys *skademlia.Keypair,
) (*CLI, error) {
	completer := readline.NewPrefixCompleter(
		readline.PcItem("status"),
		readline.PcItem("pay"),
		readline.PcItem("call"),
		readline.PcItem(
			"info",
			readline.PcItem("wallet"),
			readline.PcItem("transaction"),
		),
		readline.PcItem("spawn"),
		readline.PcItem(
		"stake",
			readline.PcItem("place"),
			readline.PcItem("withdraw"),
		),
		readline.PcItem("help"),
	)

	rl, err := readline.NewEx(
		&readline.Config{
			Prompt: "\033[31m»»»\033[0m ",
			AutoComplete: completer,
			InterruptPrompt: "^C",
			EOFPrompt:       "exit",
			HistorySearchFold: true,
		},
	)
	if err != nil {
		return nil, err
	}

	log.Set(log.NewConsoleWriter(rl.Stderr(), log.FilterFor(log.ModuleNode, log.ModuleConsensus, log.ModuleMetrics)))

	return &CLI{
		rl: rl,
		ledger: ledger,
		logger: log.Node(),
		tree: completer.Tree("    "),
		keys: keys,
	}, nil
}

func toCMD(in string, n int) []string {
	return strings.Split(strings.TrimSpace(in[n:]), " ")
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

		line = strings.TrimSpace(line)
		switch {
		case line == "status":
			cli.status()
		case strings.HasPrefix(line, "pay"):
			cli.pay(toCMD(line, 3), nil)
		case strings.HasPrefix(line, "call"):
			cli.call(toCMD(line, 4))
		case line == "info":
			fmt.Println("info [wallet [address] | transaction <transaction-id>]")
		case strings.HasPrefix(line, "info "):
			switch {
			case strings.HasPrefix(line[5:], "wallet"):
				cli.wallet(toCMD(line, 11))
			case strings.HasPrefix(line[17:], "transaction "):
			default:
				fmt.Println("one of 'wallet' or 'transaction' expected")
			}
		case line == "":
		case line == "help":
			cli.usage()
		default:
			fmt.Printf("unrecognised command :%s\n", line)
		}
	}
}

func (cli *CLI) usage() {
	_, _ = io.WriteString(cli.rl.Stderr(), "commands:\n")
	_, _ = io.WriteString(cli.rl.Stderr(), cli.tree)
}

func (cli *CLI) status() {
	round := cli.ledger.LastRound()

	cli.logger.Info().
		Uint8("difficulty", round.ExpectedDifficulty(sys.MinDifficulty, sys.DifficultyScaleFactor)).
		Uint64("round", round.Index).
		Hex("root_id", round.End.ID[:]).
		Uint64("height", cli.ledger.Height()).
		//Uint64("num_tx", ledger.NumTransactions()).
		//Uint64("num_tx_in_store", ledger.NumTransactionInStore()).
		//Uint64("num_missing_tx", ledger.NumMissingTransactions()).
		Msg("Here is the current state of the ledger.")
}

func (cli *CLI) pay(cmd []string, additional []byte) {
	if len(cmd) < 2 {
		fmt.Println("pay <recipient> <amount>")
		return
	}

	recipient, err := hex.DecodeString(cmd[0])
	if err != nil {
		cli.logger.Error().Err(err).Msg("The recipient you specified is invalid.")
		return
	}

	amount, err := strconv.Atoi(cmd[1])
	if err != nil {
		cli.logger.Error().Err(err).Msg("Failed to convert payment amount to an uint64.")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	payload.Write(recipient[:])
	binary.LittleEndian.PutUint64(intBuf[:], uint64(amount))
	payload.Write(intBuf[:])

	if additional != nil {
		payload.Write(additional)
	}

	tx := wavelet.AttachSenderToTransaction(
		cli.keys,
		wavelet.NewTransaction(cli.keys, sys.TagTransfer, payload.Bytes()),
		cli.ledger.Graph().FindEligibleParents()...,
	)

	if err := cli.ledger.AddTransaction(tx); err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
		cli.logger.Error().Msgf("error adding tx to graph [%v]: %+v\n", err, tx)
	}
}

func (cli *CLI) call(cmd []string) {
	if len(cmd) < 4 {
		fmt.Println("call <smart-contract-address> <amount> <function> [function parameters]")
		return
	}

	var intBuf [8]byte
	payload := bytes.NewBuffer(nil)
	binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(cmd[2])))
	payload.Write(intBuf[:3])
	payload.WriteString(cmd[2])

	params := bytes.NewBuffer(nil)

	for i := 3; i < len(cmd); i++ {
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

	cli.pay(cmd[:2], payload.Bytes())
}

func (cli *CLI) wallet(cmd []string) {
	if len(cmd) != 1 {
		fmt.Println("wallet [address]")
		return
	}

	snapshot := cli.ledger.Snapshot()
	publicKey := cli.keys.PublicKey()

	address := cmd[0]

	if address == "" {
		balance, _ := wavelet.ReadAccountBalance(snapshot, publicKey)
		stake, _ := wavelet.ReadAccountStake(snapshot, publicKey)
		nonce, _ := wavelet.ReadAccountNonce(snapshot, publicKey)

		cli.logger.Info().
			Str("id", hex.EncodeToString(publicKey[:])).
			Uint64("balance", balance).
			Uint64("stake", stake).
			Uint64("nonce", nonce).
			Msg("Here is your wallet information.")

		return
	}

	buf, err := hex.DecodeString(address)

	if err != nil || len(buf) != wavelet.SizeAccountID {
		cli.logger.Error().Msg("The account ID you specified is invalid.")
		return
	}

	var accountID wavelet.AccountID
	copy(accountID[:], buf)

	balance, _ := wavelet.ReadAccountBalance(snapshot, accountID)
	stake, _ := wavelet.ReadAccountStake(snapshot, accountID)
	nonce, _ := wavelet.ReadAccountNonce(snapshot, accountID)

	_, isContract := wavelet.ReadAccountContractCode(snapshot, accountID)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, accountID)

	cli.logger.Info().
		Uint64("balance", balance).
		Uint64("stake", stake).
		Uint64("nonce", nonce).
		Bool("is_contract", isContract).
		Uint64("num_pages", numPages).
		Msgf("Account: %s", cmd[1])
}