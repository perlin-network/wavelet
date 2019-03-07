package main

import (
	"fmt"
	logger "github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

func main() {
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(logger.NewConsoleWriter())

	build()

	var nodes []*node
	port := nextAvailablePort()

	nodes = append(nodes, spawn(port, nextAvailablePort(), false))

	for i := 0; i < 3; i++ {
		nodes = append(nodes, spawn(nextAvailablePort(), nextAvailablePort(), true, fmt.Sprintf("127.0.0.1:%d", port)))
	}

	wait(nodes...)

	fmt.Println("Nodes are initialized!")

	for i := 0; i < 1000; i++ {
		res, err := nodes[0].client.SendTransaction(sys.TagNop, nil)
		if err != nil {
			log.Error().Err(err).Msg("Got an error sending a transaction.")
		}

		log.Info().Str("tx_id", res.ID).Msg("Sent a new transaction.")
	}

	kill(nodes...)
}
