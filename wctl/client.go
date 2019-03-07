package wctl

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/perlin-network/noise/identity/ed25519"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

type Config struct {
	APIHost       string
	APIPort       uint16
	RawPrivateKey common.PrivateKey
	UseHTTPS      bool
}

type Client struct {
	Config
	*ed25519.Keypair

	SessionToken string
}

func NewClient(config Config) (*Client, error) {
	keys := ed25519.LoadKeys(config.RawPrivateKey[:])

	return &Client{Config: config, Keypair: keys}, nil
}

// Request will make a request to a given path, with a given body and return result in out.
func (c *Client) Request(path string, body, out interface{}) error {
	protocol := "http"
	if c.Config.UseHTTPS {
		protocol = "https"
	}

	u, err := url.Parse(fmt.Sprintf("%s://%s:%d%s", protocol, c.Config.APIHost, c.Config.APIPort, path))
	if err != nil {
		return err
	}

	req := &http.Request{
		Method: "POST",
		URL:    u,
		Header: map[string][]string{
			"Content-Type":         {"application/json"},
			api.HeaderSessionToken: {c.SessionToken},
		},
	}

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}

		req.Body = ioutil.NopCloser(bytes.NewReader(raw))
	}

	client := new(http.Client)

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		_ = res.Body.Close()
	}()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.Errorf("got an error code %v: %v", res.Status, string(data))
	}

	if out == nil {
		return nil
	}

	return json.Unmarshal(data, out)
}

// Init instantiates a new session with the Wavelet nodes HTTP API.
func (c *Client) Init() error {
	var res SessionInitResponse

	time := time.Now().UnixNano() * 1000
	message := []byte(fmt.Sprintf("%s%d", SessionInitMessage, time))

	signature, err := eddsa.Sign(c.PrivateKey(), message)
	if err != nil {
		return errors.Wrap(err, "failed to sign session init message")
	}

	req := SessionInitRequest{
		PublicKey:  hex.EncodeToString(c.PublicKey()),
		TimeMillis: uint64(time),
		Signature:  hex.EncodeToString(signature),
	}

	if err := c.Request(RouteSessionInit, &req, &res); err != nil {
		return err
	}

	c.SessionToken = res.Token

	return nil
}

func (c *Client) SendTransaction(tag byte, payload []byte) (SendTransactionResponse, error) {
	var res SendTransactionResponse

	signature, err := eddsa.Sign(c.PrivateKey(), append([]byte{tag}, payload...))
	if err != nil {
		return res, errors.Wrap(err, "failed to sign send transaction message")
	}

	req := SendTransactionRequest{
		Sender:    hex.EncodeToString(c.PublicKey()),
		Tag:       tag,
		Payload:   hex.EncodeToString(payload),
		Signature: hex.EncodeToString(signature),
	}

	if err := c.Request(RouteTxSend, &req, &res); err != nil {
		return res, err
	}

	return res, nil
}
