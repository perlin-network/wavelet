package main

import (
	"fmt"
	logger "github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.uber.org/atomic"
	"os"
	"time"
)

func main() {
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(logger.NewConsoleWriter())

	build()

	var nodes []*node
	port := nextAvailablePort()

	nodes = append(nodes, spawn(port, nextAvailablePort(), false))

	for i := 0; i < 1; i++ {
		nodes = append(nodes, spawn(nextAvailablePort(), nextAvailablePort(), true, fmt.Sprintf("127.0.0.1:%d", port)))
	}

	wait(nodes...)

	fmt.Println("Nodes are initialized!")

	tps := atomic.NewUint64(0)

	go func() {
		for range time.Tick(1 * time.Second) {
			log.Info().Uint64("tps", tps.Swap(0)).Int("num_nodes", len(nodes)).Msg("Benchmarking...")
		}
	}()

	for {
		for _, node := range nodes {
			_, err := node.client.SendTransaction(sys.TagNop, nil)
			if err != nil {
				fmt.Println(err)
				continue
			}

			tps.Add(1)
		}
	}

	kill(nodes...)
}
