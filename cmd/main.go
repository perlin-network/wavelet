package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/security"
	"os"
	"os/signal"
)

var nonce = uint64(0)

func sendTransaction(ledger *wavelet.Ledger, wallet *wavelet.Wallet, tag string, payload []byte) {
	parents, err := ledger.FindEligibleParents()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to find eligible parents.")
	}

	// Comment if you're testing for conflicts.
	nonce, err := wallet.NextNonce(ledger)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to figure out the next available nonce from our wallet.")
	}

	wired := &wire.Transaction{
		Sender:  wallet.PublicKeyHex(),
		Nonce:   nonce,
		Parents: parents,
		Tag:     tag,
		Payload: payload,
	}

	// Uncomment if you're testing for conflicts.
	//nonce++

	encoded, err := wired.Marshal()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to marshal wired transaction.")
	}

	wired.Signature = security.Sign(wallet.PrivateKey, encoded)

	id, successful, err := ledger.RespondToQuery(wired)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to respond to query.")
	}

	tx, err := ledger.GetBySymbol(id)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to find wired transaction in the database.")
	}

	err = ledger.HandleSuccessfulQuery(tx)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to process the wired transaction should it be successfully queried.")
	}

	log.Debug().Str("id", id).Interface("tx", wired).Msgf("Received a transaction, and voted '%t' for it.", successful)
}

func main() {
	keys, err := crypto.FromPrivateKey(security.SignaturePolicy, "a6a193b4665b03e6df196ab7765b04a01de00e09c4a056f487019b5e3565522fd6edf02c950c6e091cd2450552a52febbb3d29b38c22bb89b0996225ef5ec972")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to decode private key.")
	}

	wavelet := node.NewPlugin(node.Options{DatabasePath: "testdb", ServicesPath: "services"})

	builder := network.NewBuilder()

	builder.SetKeys(keys)
	builder.SetAddress("tcp://127.0.0.1:3000")

	builder.AddPlugin(new(discovery.Plugin))
	builder.AddPlugin(wavelet)

	net, err := builder.Build()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize networking.")
	}

	go net.Listen()

	net.BlockUntilListening()

	exit := make(chan os.Signal, 1)
	signal.Notify(exit, os.Interrupt)

	go func() {
		<-exit

		net.Close()
		os.Exit(0)
	}()

	reader := bufio.NewReader(os.Stdout)

	for i := 0; ; i++ {
		fmt.Print("Enter a message: ")

		bytes, _, err := reader.ReadLine()
		if err != nil {
			panic(err)
		}

		switch string(bytes) {
		case "wallet":
			log.Info().
				Str("id", hex.EncodeToString(wavelet.Wallet.PublicKey)).
				Uint64("nonce", wavelet.Wallet.CurrentNonce()).
				Uint64("balance", wavelet.Wallet.GetBalance(wavelet.Ledger)).
				Msg("Here is your wallet information.")
		case "pay":
			transfer := struct {
				Recipient string `json:"recipient"`
				Amount    uint64 `json:"amount"`
			}{"71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858", 1}

			payload, err := json.Marshal(transfer)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to marshal transfer payload.")
			}

			sendTransaction(wavelet.Ledger, wavelet.Wallet, "transfer", payload)
		default:
			sendTransaction(wavelet.Ledger, wavelet.Wallet, "nop", nil)
		}
	}
}
