package main

import (
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
)

func connectToAPI(host string, port uint16, privateKey edwards25519.PrivateKey) (*wctl.Client, error) {
	config := wctl.Config{
		APIHost:    host,
		APIPort:    port,
		PrivateKey: privateKey,
	}

	client, err := wctl.NewClient(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a new http api client")
	}

	// Attempt to instantiate a session 100 times max.

	for i := 0; i < 100; i++ {
		if err = client.Init(); err == nil {
			break
		}
	}

	if len(client.SessionToken) == 0 {
		return nil, errors.New("failed to init session with HTTP API")
	}

	return client, nil
}
