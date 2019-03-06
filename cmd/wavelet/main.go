package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/sys"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultC2 = 16
	DefaultC1 = 16
)

func main() {
	hostFlag := flag.String("h", "127.0.0.1", "host to listen for peers on")
	portFlag := flag.Uint("p", 3000, "port to listen for peers on")
	walletFlag := flag.String("w", "config/wallet.txt", "path to file containing hex-encoded private key")
	apiFlag := flag.Int("api", 0, "port to host HTTP API on")

	flag.Parse()

	log.Register(log.NewConsoleWriter(log.FilterFor("node", "sync", "contract")))
	logger := log.Node()

	var keys identity.Keypair

	privateKey, err := ioutil.ReadFile(*walletFlag)
	if err != nil {
		logger.Warn().Msgf("Could not find an existing wallet at %q. Generating a new wallet...", *walletFlag)

		keys = skademlia.NewKeys(DefaultC1, DefaultC2)

		logger.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Generated a wallet.")
	} else {
		n, err := hex.Decode(privateKey, privateKey)
		if err != nil {
			logger.Fatal().Err(err).Msgf("Failed to decode your private key from %q.", *walletFlag)
		}

		keys, err = skademlia.LoadKeys(privateKey[:n], DefaultC1, DefaultC2)
		if err != nil {
			logger.Fatal().Err(err).Msgf("The private key specified in %q is invalid.", *walletFlag)
		}

		logger.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Loaded wallet.")
	}

	params := noise.DefaultParams()
	params.Keys = keys
	params.Host = *hostFlag
	params.Port = uint16(*portFlag)
	params.MaxMessageSize = 4 * 1024 * 1024
	params.SendMessageTimeout = 1 * time.Second

	n, err := noise.NewNode(params)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to start listening for peers.")
	}

	n.OnPeerInit(func(node *noise.Node, peer *noise.Peer) error {
		peer.OnConnError(func(node *noise.Node, peer *noise.Peer, err error) error {
			logger.Info().Err(err).Msgf("An error occurred over the wire.")

			return nil
		})

		peer.OnDisconnect(func(node *noise.Node, peer *noise.Peer) error {
			logger.Info().Msgf("Peer %v has disconnected.", peer.RemoteIP().String()+":"+strconv.Itoa(int(peer.RemotePort())))

			// TODO(kenta): don't immediately evict from table
			skademlia.Table(node).Delete(protocol.PeerID(peer))
			return nil
		})

		return nil
	})

	protocol.New().
		Register(ecdh.New()).
		Register(aead.New()).
		Register(skademlia.New().WithC1(DefaultC1).WithC2(DefaultC2)).
		Register(node.New()).
		Enforce(n)

	go n.Listen()

	logger.Info().Uint16("port", n.ExternalPort()).Msg("Listening for peers.")

	if len(flag.Args()) > 0 {
		for _, address := range flag.Args() {
			peer, err := n.Dial(address)
			if err != nil {
				logger.Fatal().Err(err).Msg("Failed to dial specified peer.")
			}

			node.WaitUntilAuthenticated(peer)
		}

		peers := skademlia.FindNode(n, protocol.NodeID(n).(skademlia.ID), skademlia.BucketSize(), 8)
		logger.Info().Msgf("Bootstrapped with peers: %+v", peers)
	}

	if port := *apiFlag; port > 0 {
		go api.New().StartHTTP(n, port)
	}

	reader := bufio.NewReader(os.Stdin)

	var nodeID common.AccountID
	copy(nodeID[:], n.Keys.PublicKey())

	for {
		bytes, _, err := reader.ReadLine()

		if err != nil {
			continue
		}

		ledger := node.Ledger(n)
		cmd := strings.Split(string(bytes), " ")

		switch cmd[0] {
		case "l":
			logger.Info().
				Uint64("difficulty", ledger.Difficulty()).
				Uint64("view_id", ledger.ViewID()).
				Hex("root_id", ledger.Root().ID[:]).
				Uint64("height", ledger.Height()).
				Msg("Here is the current state of the ledger.")
		case "tx":
			if len(cmd) < 2 {
				logger.Error().Msg("Please specify a transaction ID.")
			}

			buf, err := hex.DecodeString(cmd[1])

			if err != nil || len(buf) != common.SizeTransactionID {
				logger.Error().Msg("The transaction ID you specified is invalid.")
				continue
			}

			var id common.TransactionID
			copy(id[:], buf)

			tx, exists := ledger.FindTransaction(id)
			if !exists {
				logger.Error().Msg("Could not find transaction in the ledger.")
				continue
			}

			var parents []string
			for _, parentID := range tx.ParentIDs {
				parents = append(parents, hex.EncodeToString(parentID[:]))
			}

			logger.Info().
				Strs("parents", parents).
				Hex("accounts_merkle_root", tx.AccountsMerkleRoot[:]).
				Uints64("difficulty_timestamps", tx.DifficultyTimestamps).
				Hex("sender", tx.Sender[:]).
				Hex("creator", tx.Creator[:]).
				Uint8("tag", tx.Tag).
				Uint64("timestamp", tx.Timestamp).
				Msgf("Transaction: %s", cmd[1])
		case "w":
			if len(cmd) < 2 {
				balance, _ := ledger.ReadAccountBalance(nodeID)
				stake, _ := ledger.ReadAccountStake(nodeID)

				logger.Info().
					Str("id", hex.EncodeToString(n.Keys.PublicKey())).
					Uint64("balance", balance).
					Uint64("stake", stake).
					Msg("Here is your wallet information.")

				continue
			}

			buf, err := hex.DecodeString(cmd[1])

			if err != nil || len(buf) != common.SizeAccountID {
				logger.Error().Msg("The account ID you specified is invalid.")
				continue
			}

			var accountID common.AccountID
			copy(accountID[:], buf)

			balance, _ := ledger.ReadAccountBalance(accountID)
			stake, _ := ledger.ReadAccountStake(accountID)

			logger.Info().
				Uint64("balance", balance).
				Uint64("stake", stake).
				Msgf("Account: %s", cmd[1])
		case "p":
			recipientAddress := "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405"
			amount := 1

			if len(cmd) >= 2 {
				recipientAddress = cmd[1]
			}

			if len(cmd) >= 3 {
				amount, err = strconv.Atoi(cmd[2])
				if err != nil {
					logger.Error().Err(err).Msg("Failed to convert payment amount to an uint64.")
					continue
				}
			}

			recipient, err := hex.DecodeString(recipientAddress)
			if err != nil {
				logger.Error().Err(err).Msg("The recipient you specified is invalid.")
				continue
			}

			params := payload.NewWriter(nil)

			params.WriteBytes(recipient)
			params.WriteUint64(uint64(amount))

			if len(cmd) >= 5 {
				params.WriteString(cmd[3])

				inputs := payload.NewWriter(nil)

				for i := 4; i < len(cmd); i++ {
					arg := cmd[i]

					switch arg[0] {
					case 'S':
						inputs.WriteString(arg[1:])
					case 'B':
						inputs.WriteBytes([]byte(arg[1:]))
					case '1', '2', '4', '8':
						var val uint64
						_, err = fmt.Sscanf(arg[1:], "%d", &val)
						if err != nil {
							logger.Error().Err(err).Msgf("Got an error parsing integer: %+v", arg[1:])
						}

						switch arg[0] {
						case '1':
							inputs.WriteByte(byte(val))
						case '2':
							inputs.WriteUint16(uint16(val))
						case '4':
							inputs.WriteUint32(uint32(val))
						case '8':
							inputs.WriteUint64(uint64(val))
						}
					case 'H':
						b, err := hex.DecodeString(arg[1:])
						if err != nil {
							logger.Error().Err(err).Msgf("Cannot decode hex: %s", arg[1:])
							continue
						}

						inputs.WriteBytes(b)
					default:
						logger.Error().Msgf("Invalid argument specified: %s", arg)
						continue
					}
				}

				params.WriteBytes(inputs.Bytes())
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagTransfer, params.Bytes())
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a transfer transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a transfer transaction.")
					return
				}

				logger.Info().Msgf("Success! Your payment transaction ID: %x", tx.ID)
			}()

		case "ps":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				logger.Fatal().Err(err).Msg("Failed to convert staking amount to a uint64.")
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagStake, payload.NewWriter(nil).WriteUint64(uint64(amount)).Bytes())
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a stake placement transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a stake placement transaction.")
					return
				}

				logger.Info().Msgf("Success! Your stake placement transaction ID: %x", tx.ID)
			}()
		case "ws":
			if len(cmd) < 2 {
				continue
			}

			amount, err := strconv.Atoi(cmd[1])
			if err != nil {
				logger.Fatal().Err(err).Msg("Failed to convert withdraw amount to an uint64.")
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagStake, payload.NewWriter(nil).WriteUint64(uint64(amount)).Bytes())
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a stake withdrawal transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a stake withdrawal transaction.")
					return
				}

				logger.Info().Msgf("Success! Your stake withdrawal transaction ID: %x", tx.ID)
			}()
		case "c":
			if len(cmd) < 2 {
				continue
			}

			code, err := ioutil.ReadFile(cmd[1])
			if err != nil {
				logger.Error().
					Err(err).
					Str("path", cmd[1]).
					Msg("Failed to find/load the smart contract code from the given path.")
				continue
			}

			go func() {
				tx, err := ledger.NewTransaction(n.Keys, sys.TagContract, code)
				if err != nil {
					logger.Error().Err(err).Msg("Failed to create a smart contract creation transaction.")
					return
				}

				err = node.BroadcastTransaction(n, tx)
				if err != nil {
					logger.Error().Err(err).Msg("An error occurred while broadcasting a smart contract creation transaction.")
					return
				}

				logger.Info().Msgf("Success! Your smart contract ID: %x", tx.ID)
			}()
		}
	}
}
