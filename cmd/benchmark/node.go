package main

import (
	"github.com/rs/zerolog/log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func spawn(port uint16, apiPort uint16, randomWallet bool, peers ...string) {
	cmd := exec.Command("./wavelet", "-p", strconv.Itoa(int(port)))

	if apiPort != 0 {
		cmd.Args = append(cmd.Args, "-api", strconv.Itoa(int(apiPort)))
	}

	if randomWallet {
		cmd.Args = append(cmd.Args, "-w", " ")
	}

	if len(peers) > 0 {
		cmd.Args = append(cmd.Args, strings.Join(peers, " "))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to spawn a single Wavelet node.")
	}
}
