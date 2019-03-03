package log

import (
	"encoding/hex"
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"io"
	"os"
)

var (
	output = new(multiWriter)
	logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(output)

	node        zerolog.Logger
	accounts    zerolog.Logger
	broadcaster zerolog.Logger
	consensus   zerolog.Logger
	contract    zerolog.Logger
	stake       zerolog.Logger
	tx          zerolog.Logger
)

const (
	KeyModule = "mod"
	KeyEvent  = "event"

	ModuleNode        = "node"
	ModuleAccounts    = "accounts"
	ModuleBroadcaster = "broadcaster"
	ModuleConsensus   = "consensus"
	ModuleContract    = "contract"
	ModuleStake       = "stake"
	ModuleTx          = "tx"
)

func Register(w ...io.Writer) {
	for _, writer := range w {
		output.Register(writer)
	}
}

func init() {
	setupChildLoggers()
}

func setupChildLoggers() {
	node = logger.With().Str(KeyModule, ModuleNode).Logger()
	accounts = logger.With().Str(KeyModule, ModuleAccounts).Logger()
	broadcaster = logger.With().Str(KeyModule, ModuleBroadcaster).Logger()
	consensus = logger.With().Str(KeyModule, ModuleConsensus).Logger()
	contract = logger.With().Str(KeyModule, ModuleContract).Logger()
	stake = logger.With().Str(KeyModule, ModuleStake).Logger()
	tx = logger.With().Str(KeyModule, ModuleTx).Logger()
}

func Node() zerolog.Logger {
	return node
}

func Account(id [sys.PublicKeySize]byte, event string) zerolog.Logger {
	return accounts.With().Hex("account_id", id[:]).Str(KeyEvent, event).Logger()
}

func Contracts() zerolog.Logger {
	return contract
}

func Contract(id [sys.TransactionIDSize]byte, event string) zerolog.Logger {
	return contract.With().Hex("contract_id", id[:]).Str(KeyEvent, event).Logger()
}

func Broadcaster() zerolog.Logger {
	return broadcaster
}

func TX(id [sys.TransactionIDSize]byte, sender, creator [sys.PublicKeySize]byte, parentIDs [][sys.PublicKeySize]byte, tag byte, payload []byte, event string) zerolog.Logger {
	var parents []string

	for _, parentID := range parentIDs {
		parents = append(parents, hex.EncodeToString(parentID[:]))
	}

	return tx.With().
		Hex("tx_id", id[:]).
		Hex("sender_id", sender[:]).
		Hex("creator_id", creator[:]).
		Strs("parents", parents).
		Uint8("tag", tag).
		Hex("payload", payload).
		Str(KeyEvent, event).Logger()
}

func Consensus(event string) zerolog.Logger {
	return consensus.With().Str(KeyEvent, event).Logger()
}

func Stake(event string) zerolog.Logger {
	return stake.With().Str(KeyEvent, event).Logger()
}
