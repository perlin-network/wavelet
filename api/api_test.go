package api_test

import (
	"fmt"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/wavelet/api"
	apiClient "github.com/perlin-network/wavelet/cmd/wctl/client"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

const (
	host           = "localhost"
	privateKeyFile = "../cmd/wavelet/wallet.txt"
)

func setupMockServer(port int, privateKeyFile string) (*http.Server, error) {
	privateKeyBytes, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, err
	}
	keys, err := crypto.FromPrivateKey(security.SignaturePolicy, string(privateKeyBytes))
	if err != nil {
		return nil, err
	}
	w := node.NewWaveletMock(node.MockOptions{})
	sc := make(chan *http.Server)
	go api.Run(nil, w, sc, api.Options{
		ListenAddr: fmt.Sprintf("%s:%d", "localhost", port),
		Clients: []*api.ClientInfo{
			&api.ClientInfo{
				PublicKey: keys.PublicKeyHex(),
				Permissions: api.ClientPermissions{
					CanSendTransaction: true,
					CanPollTransaction: true,
					CanControlStats:    true,
					CanAccessLedger:    true,
				},
			},
		},
	})
	server := <-sc
	time.Sleep(50 * time.Millisecond)
	return server, nil
}

func client(port int, privateKeyFile string) (*apiClient.Client, error) {
	privateKeyBytes, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to open api private key file: %s", privateKeyFile)
	}

	client, err := apiClient.NewClient(apiClient.Config{
		APIHost:    host,
		APIPort:    uint(port),
		PrivateKey: string(privateKeyBytes),
		UseHTTPS:   false,
	})
	if err != nil {
		return nil, err
	}

	err = client.Init()
	if err != nil {
		return nil, err
	}

	log.Debug().Str("SessionToken", client.SessionToken).Msg(" ")
	return client, nil
}

func Test_api_serverVersion(t *testing.T) {
	port := 30000
	s, err := setupMockServer(port, privateKeyFile)
	assert.Equal(t, nil, err)
	defer s.Close()

	c, err := client(port, privateKeyFile)
	assert.Equal(t, nil, err)

	_, err = c.ServerVersion()
	assert.Equal(t, nil, err)
}
