package main

import (
	"bufio"
	"fmt"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"os"
	"os/signal"
)

func main() {
	keys, err := crypto.FromPrivateKey(security.SignaturePolicy, "a6a193b4665b03e6df196ab7765b04a01de00e09c4a056f487019b5e3565522fd6edf02c950c6e091cd2450552a52febbb3d29b38c22bb89b0996225ef5ec972")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to decode private key.")
	}

	log.Info().
		Str("public_key", keys.PublicKeyHex()).
		Str("private_key", keys.PrivateKeyHex()).
		Msg("Keypair loaded.")

	ledger := wavelet.NewLedger()
	go ledger.ProcessTransactions()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c

		ledger.Cleanup()
		os.RemoveAll("testdb")

		os.Exit(0)
	}()

	reader := bufio.NewReader(os.Stdout)

	for i := 0; ; i++ {
		fmt.Print("Enter a message: ")

		bytes, _, err := reader.ReadLine()
		if err != nil {
			panic(err)
		}

		parents, err := ledger.FindEligibleParents()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to find eligible parents.")
		}

		wired := &wire.Transaction{
			Sender:  keys.PublicKeyHex(),
			Nonce:   uint64(i),
			Parents: parents,
			Tag:     "nop",
			Payload: bytes,
		}

		encoded, err := wired.Marshal()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to marshal wired transaction.")
		}

		wired.Signature = security.Sign(keys.PrivateKey, encoded)

		id, successful, err := ledger.RespondToQuery(wired)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to respond to query.")
		}

		log.Debug().Str("id", id).Interface("tx", wired).Msgf("Received a transaction, and voted '%t' for it.", successful)
	}
}
