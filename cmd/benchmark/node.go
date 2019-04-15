package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

var (
	LoadedWallet      = "Loaded wallet."
	GeneratedWallet   = "Generated a wallet."
	StartedAPI        = "Started HTTP API server."
	ListeningForPeers = "Listening for peers."
)

var _ io.Writer = (*node)(nil)

type node struct {
	keys   *skademlia.Keypair
	client *wctl.Client

	cmd               *exec.Cmd
	host              string
	nodePort, apiPort uint16

	nodeReady, apiReady chan struct{}
}

func (n *node) Write(buf []byte) (num int, err error) {
	var fields map[string]interface{}

	decoder := json.NewDecoder(bytes.NewReader(buf))
	decoder.UseNumber()

	err = decoder.Decode(&fields)
	if err != nil {
		return num, errors.Wrapf(err, "cannot decode field: %q", err)
	}

	if val, exists := fields["error"]; exists {
		if err, ok := val.(string); ok {
			log.Error().
				Str("node", fmt.Sprintf("%s:%d", n.host, n.nodePort)).
				Str("error", err).
				Msg("Node reported an error.")
		}
	}

	if msg, exists := fields["message"]; exists {
		if msg, ok := msg.(string); ok {
			err = n.parseMessage(fields, msg)

			if err != nil {
				fmt.Printf("%v\n", err)
				return num, errors.Wrap(err, "failed to parse message")
			}
		}
	}

	return len(buf), nil
}

func (n *node) parseMessage(fields map[string]interface{}, msg string) error {
	switch msg {
	case GeneratedWallet:
		fallthrough
	case LoadedWallet:
		privateKeyBuf, err := hex.DecodeString(fields["privateKey"].(string))
		if err != nil {
			return errors.Wrap(err, "failed to decode nodes private key")
		}

		var privateKey edwards25519.PrivateKey
		copy(privateKey[:], privateKeyBuf)

		n.keys, err = skademlia.LoadKeys(privateKey, sys.SKademliaC1, sys.SKademliaC2)
		if err != nil {
			return errors.Wrap(err, "nodes private keyis invalid")
		}
	case StartedAPI:
		if n.keys == nil {
			return errors.New("started api before reading wallet keys")
		}

		if err := n.init(); err != nil {
			return errors.Wrap(err, "failed to init wavelet node")
		}
	case ListeningForPeers:
		close(n.nodeReady)
	default:
	}

	return nil
}

// wait waits until the node is fully initialized and ready for commanding.
func (n *node) wait() {
	<-n.apiReady
}

// kill kills the nodes process.
func (n *node) kill() {
	_ = n.cmd.Process.Kill()
}

func (n *node) init() error {
	var err error

	if n.client, err = connectToAPI("127.0.0.1", n.apiPort, n.keys.PrivateKey()); err != nil {
		return err
	}

	publicKey := n.keys.PublicKey()

	log.Info().
		Uint16("node_port", n.nodePort).
		Uint16("api_port", n.apiPort).
		Hex("public_key", publicKey[:]).
		Msg("Spawned a new Wavelet node.")

	close(n.apiReady)

	return nil
}

func spawn(nodePort, apiPort uint16, randomWallet bool, peers ...string) *node {
	cmd := exec.Command("./wavelet", "-port", strconv.Itoa(int(nodePort)))

	if apiPort != 0 {
		cmd.Args = append(cmd.Args, "-api.port", strconv.Itoa(int(apiPort)))
	}

	if randomWallet {
		cmd.Args = append(cmd.Args, "-wallet", "random")
	}

	if len(peers) > 0 {
		cmd.Args = append(cmd.Args, strings.Join(peers, " "))
	}

	// TODO(kenta): allow external hosts
	n := &node{
		cmd: cmd,

		host: "127.0.0.1",

		nodePort: nodePort,
		apiPort:  apiPort,

		nodeReady: make(chan struct{}),
		apiReady:  make(chan struct{}),
	}

	cmd.Stdout = n
	cmd.Stderr = n

	if err := cmd.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to spawn a single Wavelet node.")
	}

	<-n.nodeReady

	return n
}
