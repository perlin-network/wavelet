package main

import (
	logger "github.com/perlin-network/wavelet/log"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

func main() {
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger().Output(logger.NewConsoleWriter())

	build()

	spawn(nextAvailablePort(), nextAvailablePort())
	spawn(nextAvailablePort(), nextAvailablePort())
	spawn(nextAvailablePort(), nextAvailablePort())

	select {}
}
