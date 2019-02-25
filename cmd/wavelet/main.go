package main

import (
	"bufio"
	"encoding/hex"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/identity"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"io/ioutil"
	"os"
)

const DefaultC1, DefaultC2 = 16, 16

func main() {
	walletPath := "config/wallet.txt"
	genesisPath := "config/genesis.json"

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(kv, genesisPath)
	ledger.RegisterProcessor(sys.TagNop, new(wavelet.NopProcessor))
	ledger.RegisterProcessor(sys.TagTransfer, new(wavelet.TransferProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(wavelet.StakeProcessor))

	var keys identity.Keypair

	privateKey, err := ioutil.ReadFile(walletPath)
	if err != nil {
		log.Warn().Msgf("Could not find an existing wallet at %q. Generating a new wallet...", walletPath)

		keys = skademlia.NewKeys(DefaultC1, DefaultC2)

		log.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Generated a wallet.")
	} else {
		n, err := hex.Decode(privateKey, privateKey)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to decode your private key from %q.", walletPath)
		}

		keys, err = skademlia.LoadKeys(privateKey[:n], DefaultC1, DefaultC2)
		if err != nil {
			log.Fatal().Err(err).Msgf("The private key specified in %q is invalid.", walletPath)
		}

		log.Info().
			Hex("privateKey", keys.PrivateKey()).
			Hex("publicKey", keys.PublicKey()).
			Msg("Loaded wallet.")
	}

	params := noise.DefaultParams()
	params.Keys = keys

	node, err := noise.NewNode(params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to start listening for peers.")
	}

	protocol.New().
		Register(ecdh.New()).
		Register(aead.New()).
		Register(skademlia.New().WithC1(DefaultC1).WithC2(DefaultC2)).
		Enforce(node)

	go node.Listen()

	log.Info().Uint16("port", node.ExternalPort()).Msg("Listening for peers.")

	reader := bufio.NewReader(os.Stdin)

	for {
		_, err := reader.ReadString('\n')

		if err != nil {
			log.Fatal().Err(err).Msg("Failed to read input from user.")
		}
	}
}
