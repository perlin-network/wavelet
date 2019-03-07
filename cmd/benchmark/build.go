package main

import (
	"github.com/rs/zerolog/log"
	"os/exec"
)

func build() {
	log.Debug().Msg("Building Wavelet...")

	cmd := exec.Command("go", "build", "../wavelet/")

	if err := cmd.Run(); err != nil {
		log.Fatal().Err(err).Msg("Failed to build Wavelet node executable.")
	}

	log.Info().Msg("Successfully built Wavelet.")
}
