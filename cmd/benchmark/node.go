package main

import (
	"github.com/rs/zerolog/log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func spawn(port uint16, peers ...string) {
	var cmd *exec.Cmd
	if len(peers) > 0 {
		cmd = exec.Command("./wavelet", "-p", strconv.Itoa(int(port)), strings.Join(peers, " "))
	} else {
		cmd = exec.Command("./wavelet", "-p", strconv.Itoa(int(port)))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to spawn a single Wavelet node.")
	}
}
