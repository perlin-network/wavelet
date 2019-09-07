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
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/valyala/fastjson"
)

const (
	RouteLedger   = "/ledger"
	RouteAccount  = "/accounts"
	RouteContract = "/contract"
	RouteTxList   = "/tx"
	RouteTxSend   = "/tx/send"

	RouteWSBroadcaster  = "/poll/broadcaster"
	RouteWSConsensus    = "/poll/consensus"
	RouteWSStake        = "/poll/stake"
	RouteWSAccounts     = "/poll/accounts"
	RouteWSContracts    = "/poll/contract"
	RouteWSTransactions = "/poll/tx"
	RouteWSMetrics      = "/poll/metrics"

	ReqPost = "POST"
	ReqGet  = "GET"
)

type Marshalable interface {
	Marshal() []byte
}

type Config struct {
	APIHost    string
	APIPort    uint16
	PrivateKey edwards25519.PrivateKey
	UseHTTPS   bool
	Timeout    time.Duration
}

type Client struct {
	Config

	stdClient *http.Client

	edwards25519.PrivateKey
	edwards25519.PublicKey

	jsonPool fastjson.ParserPool

	// TODO: metrics, stake, consensus, network

	// These callbacks are only called when the function that calls them is
	// started. The function's name is written above each block.
	// These functions would also return a callback to stop polling.

	// Websocket callbacks
	OnError

	OnBalanceUpdated
	OnNumPagesUpdated
	OnStakeUpdated
	OnRewardUpdated

	OnPeerJoin
	OnPeerLeave

	OnRoundEnd
	OnPrune

	OnStake

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

	c := &Client{
		Config:     config,
		PrivateKey: config.PrivateKey,
		PublicKey:  config.PrivateKey.Public(),
		stdClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		OnError: func(err error) {
			log.Println("WCTL_ERR:", err)
		},
	}

	return c, nil
}

/*
func (c *Client) PollLoggerSink(sinkRoute string) (<-chan []byte, error) {
	if err := c.pollWS(stop, evChan, sinkRoute, nil); err != nil {
		return nil, err
	}

	return evChan, nil
}

// PollAccounts queries
func (c *Client) PollAccounts(accountID string) (<-chan []byte, error) {
	v := url.Values{}
	if accountID != "" {
		v.Set("id", accountID)
	}

	evChan := make(chan []byte)

	if err := c.pollWS(stop, evChan, RouteWSAccounts, v); err != nil {
		return nil, err
	}

	return evChan, nil
}

func (c *Client) PollContracts(stop <-chan struct{}, contractID string) (<-chan []byte, error) {
	v := url.Values{}
	if contractID != "" {
		v.Set("id", contractID)
	}

	evChan := make(chan []byte)

	if err := c.pollWS(stop, evChan, RouteWSContracts, v); err != nil {
		return nil, err
	}

	return evChan, nil
}
*/

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
