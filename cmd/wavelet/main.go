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
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/net"
	"github.com/perlin-network/wavelet/sys"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

const DefaultC1, DefaultC2 = 16, 16

func main() {
	hostFlag := flag.String("h", "127.0.0.1", "host to listen for peers on")
	portFlag := flag.Uint("p", 3000, "port to listen for peers on")
	walletFlag := flag.String("w", "config/wallet.txt", "path to file containing hex-encoded private key")
	flag.Parse()

	var keys identity.Keypair

	privateKey, err := ioutil.ReadFile(*walletFlag)
	if err != nil {
		log.Warn().Msgf("Could not find an existing wallet at %q. Generating a new wallet...", *walletFlag)

		keys = skademlia.NewKeys(DefaultC1, DefaultC2)

		log.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Generated a wallet.")
	} else {
		n, err := hex.Decode(privateKey, privateKey)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to decode your private key from %q.", *walletFlag)
		}

		keys, err = skademlia.LoadKeys(privateKey[:n], DefaultC1, DefaultC2)
		if err != nil {
			log.Fatal().Err(err).Msgf("The private key specified in %q is invalid.", *walletFlag)
		}

		log.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Loaded wallet.")
	}

	params := noise.DefaultParams()
	params.Keys = keys
	params.Host = *hostFlag
	params.Port = uint16(*portFlag)

	node, err := noise.NewNode(params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start listening for peers.")
	}

	protocol.New().
		Register(ecdh.New()).
		Register(aead.New()).
		Register(skademlia.New().WithC1(DefaultC1).WithC2(DefaultC2)).
		Register(net.New()).
		Enforce(node)

	node.OnPeerInit(func(node *noise.Node, peer *noise.Peer) error {
		peer.OnConnError(func(node *noise.Node, peer *noise.Peer, err error) error {
			log.Info().Msgf("Got an error: %v", err)

			return nil
		})

		peer.OnDisconnect(func(node *noise.Node, peer *noise.Peer) error {
			log.Info().Msgf("Peer %v has disconnected.", peer.RemoteIP().String()+":"+strconv.Itoa(int(peer.RemotePort())))

			return nil
		})

		return nil
	})

	go node.Listen()

	log.Info().Uint16("port", node.ExternalPort()).Msg("Listening for peers.")

	if len(flag.Args()) > 0 {
		for _, address := range flag.Args() {
			peer, err := node.Dial(address)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to dial specified peer.")
			}

			net.WaitUntilAuthenticated(peer)
		}

		peers := skademlia.FindNode(node, protocol.NodeID(node).(skademlia.ID), skademlia.BucketSize(), 8)
		log.Info().Msgf("Bootstrapped with peers: %+v", peers)
	}

	reader := bufio.NewReader(os.Stdin)

	var nodeID [wavelet.PublicKeySize]byte
	copy(nodeID[:], node.Keys.PublicKey())

	for {
		bytes, _, err := reader.ReadLine()

		if err != nil {
			log.Fatal().Err(err).Msg("Failed to read input from user.")
		}

		ledger := net.Ledger(node)
		cmd := strings.Split(string(bytes), " ")

		switch cmd[0] {
		case "w":
			if len(cmd) < 2 {
				balance, _ := ledger.ReadAccountBalance(nodeID)
				stake, _ := ledger.ReadAccountStake(nodeID)

				log.Info().
					Str("id", hex.EncodeToString(node.Keys.PublicKey())).
					Uint64("balance", balance).
					Uint64("stake", stake).
					Msg("Here is your wallet information.")

				continue
			}

			buf, err := hex.DecodeString(cmd[1])

			if err != nil || len(buf) != wavelet.PublicKeySize {
				log.Error().Msg("The account ID you specified is invalid.")
				continue
			}

			var accountID [wavelet.PublicKeySize]byte
			copy(accountID[:], buf)

			balance, _ := ledger.ReadAccountBalance(accountID)
			stake, _ := ledger.ReadAccountStake(accountID)

			log.Info().
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
					log.Error().Err(err).Msg("Failed to convert payment amount to an uint64.")
					continue
				}
			}

			recipient, err := hex.DecodeString(recipientAddress)
			if err != nil {
				log.Error().Err(err).Msg("The recipient you specified is invalid.")
				continue
			}

			payload := payload.NewWriter(recipient)
			payload.WriteUint64(uint64(amount))

			if len(cmd) >= 5 {
				payload.WriteString(cmd[3])

				for i := 4; i < len(cmd); i++ {
					arg := cmd[i]

					switch arg[0] {
					case 'S':
						payload.WriteString(arg[1:])
					case 'B':
						payload.WriteBytes([]byte(arg[1:]))
					case '1', '2', '4', '8':
						var val uint64
						fmt.Sscanf(arg[1:], "%d", &val)

						switch arg[0] {
						case '1':
							payload.WriteByte(byte(val))
						case '2':
							payload.WriteUint16(uint16(val))
						case '4':
							payload.WriteUint32(uint32(val))
						case '8':
							payload.WriteUint64(uint64(val))
						}
					case 'H':
						b, err := hex.DecodeString(arg[1:])
						if err != nil {
							log.Error().Err(err).Msgf("Cannot decode hex: %s", arg[1:])
							continue
						}

						payload.WriteBytes(b)
					default:
						log.Error().Msgf("Invalid argument specified: %s", arg)
						continue
					}
				}
			}

			tx, err := ledger.NewTransaction(node.Keys, sys.TagTransfer, payload.Bytes())
			if err != nil {
				log.Error().Err(err).Msg("Failed to create a transfer transaction.")
				continue
			}

			err = net.BroadcastTransaction(node, tx)
			if err != nil {
				log.Error().Err(err).Msg("An error occurred while broadcasting a transfer transaction.")
				continue
			}

			log.Info().Msgf("Success! Your payment transaction ID: %x", tx.ID)
		}
	}
}
