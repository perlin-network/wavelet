// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// Package wctl provides a client wrapper for the API.
package wctl

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/cmd/wavelet/node"
	"github.com/valyala/fastjson"
)

const (
	RouteLedger   = "/ledger"
	RouteAccount  = "/accounts"
	RouteContract = "/contract"
	RouteTxList   = "/tx"
	RouteTxSend   = "/tx/send"
	RouteNonce    = "/nonce"

	RouteNode       = "/node"
	RouteConnect    = RouteNode + "/connect"
	RouteDisconnect = RouteNode + "/disconnect"
	RouteRestart    = RouteNode + "/restart"

	ReqPost = "POST"
	ReqGet  = "GET"
)

const JSONPoolSize = 8 // sane size of 8 arenas and parsers in a pool

var ErrNoHost = errors.New("No host provided")

type Marshalable interface {
	Marshal() []byte
}

type Config struct {
	APIHost    string
	APIPort    uint16
	APISecret  string
	PrivateKey edwards25519.PrivateKey
	UseHTTPS   bool
	Timeout    time.Duration

	// Optional
	Server *node.Wavelet
}

type Client struct {
	Config

	stdClient *http.Client

	edwards25519.PrivateKey
	edwards25519.PublicKey

	jsonPool fastjson.ParserPool
	url      string

	// Local state counters
	Nonce uint64
	Block uint64

	// JSON stuff
	parsers fastjson.ParserPool
	arenas  fastjson.ArenaPool

	// Stop the background consensus that is created before
	stopConsensus func()

	// Stop other websockets that the user spawned
	stopSockets []func()

	// TODO: metrics, stake, consensus, network

	// These callbacks are only called when the function that calls them is
	// started. The function's name is written above each block.
	// These functions would also return a callback to stop polling.

	// Websocket callbacks
	OnError

	// Accounts
	OnBalanceUpdated
	OnGasBalanceUpdated
	OnNumPagesUpdated
	OnStakeUpdated
	OnRewardUpdated

	// Network
	OnPeerJoin
	OnPeerLeave

	// Consensus
	OnProposal
	OnFinalized

	// Stake
	OnStakeRewardValidator

	// Contract
	OnContractGas
	OnContractLog

	// PollTransactions
	OnTxApplied
	OnTxGossipError
	OnTxFailed

	OnMetrics
}

func NewClient(config Config) (*Client, error) {
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}

	if config.APIHost == "" {
		return nil, ErrNoHost
	}

	protocol := "http"
	if config.UseHTTPS {
		protocol = "https"
	}

	c := &Client{
		Config:     config,
		PrivateKey: config.PrivateKey,
		PublicKey:  config.PrivateKey.Public(),
		url: (&url.URL{
			Scheme: protocol,
			Host:   fmt.Sprintf("%s:%d", config.APIHost, config.APIPort),
		}).String(),
		stdClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		OnError: func(err error) {
			log.Println("WCTL_ERR:", err)
		},
	}

	// Generate parsers and arenas
	for i := 0; i < JSONPoolSize; i++ {
		var a fastjson.Arena
		c.arenas.Put(&a)

		var p fastjson.Parser
		c.parsers.Put(&p)
	}

	// Get the nonce
	n, err := c.GetSelfNonce()
	if err != nil {
		return c, err
	}

	atomic.StoreUint64(&c.Nonce, n.Nonce)
	atomic.StoreUint64(&c.Block, n.Block)

	// Start listening to consensus to track Block
	cancel, err := c.pollConsensus()
	if err != nil {
		cancel()
		return nil, err
	}

	c.stopConsensus = cancel

	return c, nil
}

func (c *Client) Close() {
	c.stopConsensus()

	// cancel user-spawned sockets
	for _, c := range c.stopSockets {
		c()
	}
}

func (c *Client) GetContractCode(contractID string) (string, error) {
	path := fmt.Sprintf("%s/%s", RouteContract, contractID)

	res, err := c.Request(path, ReqGet, nil)
	return string(res), err
}

func (c *Client) GetContractPages(contractID string, index *uint64) (string, error) {
	path := fmt.Sprintf("%s/%s/page", RouteContract, contractID)
	if index != nil {
		path = fmt.Sprintf("%s/%d", path, *index)
	}

	res, err := c.Request(path, ReqGet, nil)
	return base64.StdEncoding.EncodeToString(res), err
}
