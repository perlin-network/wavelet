package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/net"
	"io/ioutil"
	"os"
	"strconv"
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

			skademlia.WaitUntilAuthenticated(peer)
		}

		peers := skademlia.FindNode(node, protocol.NodeID(node).(skademlia.ID), skademlia.BucketSize(), 8)
		log.Info().Msgf("Bootstrapped with peers: %+v", peers)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		_, err := reader.ReadString('\n')

		if err != nil {
			log.Fatal().Err(err).Msg("Failed to read input from user.")
		}
	}
}
