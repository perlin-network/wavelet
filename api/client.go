package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/perlin-network/wavelet/security"
)

// Client represents a Perlin Ledger client.
type Client struct {
	Config       ClientConfig
	SessionToken string
	KeyPair      *security.KeyPair
}

// ClientConfig represents a Perlin Ledger client config.
type ClientConfig struct {
	RemoteAddr string
	PrivateKey string
	UseHTTPS   bool
}

// SessionResponse represents the response from a session call
type SessionResponse struct {
	Token string `json:"token"`
}

// NewClient creates a new Perlin Ledger client from a config.
func NewClient(config ClientConfig) (*Client, error) {
	keys, err := security.FromPrivateKey(security.SignaturePolicy, config.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &Client{
		Config:  config,
		KeyPair: keys,
	}, nil
}

// Init will initialize a client.
func (c *Client) Init() error {
	millis := time.Now().Unix() * 1000
	authStr := fmt.Sprintf("%s%d", sessionInitSigningPrefix, millis)
	sig := security.Sign(c.KeyPair.PrivateKey, []byte(authStr))

	creds := credentials{
		PublicKey:  hex.EncodeToString(c.KeyPair.PublicKey),
		TimeMillis: millis,
		Sig:        hex.EncodeToString(sig),
	}

	resp := SessionResponse{}

	err := c.Request("/session/init", &creds, &resp)
	if err != nil {
		return err
	}
	c.SessionToken = resp.Token
	return nil
}

// Request will make a request to a given path, with a given body and return result in out.
func (c *Client) Request(path string, body, out interface{}) error {
	prot := "http"
	if c.Config.UseHTTPS {
		prot = "https"
	}
	u, err := url.Parse(fmt.Sprintf("%s://%s%s", prot, c.Config.RemoteAddr, path))
	if err != nil {
		return err
	}

	rawBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req := &http.Request{
		Method: "POST",
		URL:    u,
		Header: map[string][]string{
			"X-Session-Token": []string{c.SessionToken},
		},
		Body: ioutil.NopCloser(bytes.NewReader(rawBody)),
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("got an error code %s: %s", resp.Status, string(data))
	}

	if out == nil {
		return nil
	}
	err = json.Unmarshal(data, out)
	return err
}
