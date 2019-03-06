package main

import (
	"github.com/rs/zerolog/log"
	"net"
)

func nextAvailablePort() uint16 {
	listener, err := net.Listen("tcp", ":0")

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to find an available port on the system.")
	}

	defer listener.Close()

	return uint16(listener.Addr().(*net.TCPAddr).Port)
}
