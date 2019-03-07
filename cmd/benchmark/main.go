package main

import (
	"fmt"
	logger "github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

func main() {
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(logger.NewConsoleWriter())

	build()

	port := nextAvailablePort()

	spawn(port, nextAvailablePort(), false)

	for i := 0; i < 3; i++ {
		spawn(nextAvailablePort(), nextAvailablePort(), true, fmt.Sprintf("127.0.0.1:%d", port))
	}

	select {}
}
