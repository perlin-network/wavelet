package log

import (
	"encoding/base64"
	"encoding/hex"
	"github.com/perlin-network/wavelet/common"
	"github.com/rs/zerolog"
	"io"
)

var (
	output = new(multiWriter)
	logger = zerolog.New(output).With().Timestamp().Logger()

	node        zerolog.Logger
	network     zerolog.Logger
	accounts    zerolog.Logger
	broadcaster zerolog.Logger
	consensus   zerolog.Logger
	contract    zerolog.Logger
	syncer      zerolog.Logger
	stake       zerolog.Logger
	tx          zerolog.Logger
)

const (
	KeyModule = "mod"
	KeyEvent  = "event"

	ModuleNode        = "node"
	ModuleNetwork     = "network"
	ModuleAccounts    = "accounts"
	ModuleBroadcaster = "broadcaster"
	ModuleConsensus   = "consensus"
	ModuleContract    = "contract"
	ModuleSync        = "sync"
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
	network = logger.With().Str(KeyModule, ModuleNetwork).Logger()
	accounts = logger.With().Str(KeyModule, ModuleAccounts).Logger()
	broadcaster = logger.With().Str(KeyModule, ModuleBroadcaster).Logger()
	consensus = logger.With().Str(KeyModule, ModuleConsensus).Logger()
	contract = logger.With().Str(KeyModule, ModuleContract).Logger()
	syncer = logger.With().Str(KeyModule, ModuleSync).Logger()
	stake = logger.With().Str(KeyModule, ModuleStake).Logger()
	tx = logger.With().Str(KeyModule, ModuleTx).Logger()
}

func Node() zerolog.Logger {
	return node
}

func Network(event string) zerolog.Logger {
	return network.With().Str(KeyEvent, event).Logger()
}

func Account(id common.AccountID, event string) zerolog.Logger {
	return accounts.With().Hex("account_id", id[:]).Str(KeyEvent, event).Logger()
}

func Contracts() zerolog.Logger {
	return contract
}

func Contract(id common.TransactionID, event string) zerolog.Logger {
	return contract.With().Hex("contract_id", id[:]).Str(KeyEvent, event).Logger()
}

func Broadcaster() zerolog.Logger {
	return broadcaster
}

func TX(id common.TransactionID, sender, creator common.AccountID, nonce uint64, parentIDs []common.AccountID, tag byte, payload []byte, event string) zerolog.Logger {
	var parents []string

	for _, parentID := range parentIDs {
		parents = append(parents, hex.EncodeToString(parentID[:]))
	}

	return tx.With().
		Hex("tx_id", id[:]).
		Hex("sender_id", sender[:]).
		Hex("creator_id", creator[:]).
		Uint64("nonce", nonce).
		Strs("parents", parents).
		Uint8("tag", tag).
		Str("payload", base64.StdEncoding.EncodeToString(payload)).
		Str(KeyEvent, event).Logger()
}

func Consensus(event string) zerolog.Logger {
	return consensus.With().Str(KeyEvent, event).Logger()
}

func Stake(event string) zerolog.Logger {
	return stake.With().Str(KeyEvent, event).Logger()
}

func Sync(event string) zerolog.Logger {
	return syncer.With().Str(KeyEvent, event).Logger()
}
