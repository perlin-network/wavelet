package wctl

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/perlin-network/noise/identity/ed25519"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"net/http"
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
func (c *Client) Request(path string, method string, body, out interface{}) error {
	protocol := "http"
	if c.Config.UseHTTPS {
		protocol = "https"
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	req.URI().Update(fmt.Sprintf("%s://%s:%d%s", protocol, c.Config.APIHost, c.Config.APIPort, path))
	req.Header.SetMethod(method)
	req.Header.SetContentType("application/json")
	req.Header.Add(api.HeaderSessionToken, c.SessionToken)

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}

		req.SetBody(raw)
	}

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	if err := fasthttp.Do(req, res); err != nil {
		return err
	}

	//if code := res.StatusCode(); code != fasthttp.StatusOK {
	//	return errors.Errorf("unexpected status code: %d", code)
	//}

	if out == nil {
		return nil
	}

	return json.Unmarshal(res.Body(), out)
}

// EstablishWS will create a websocket connection.
func (c *Client) EstablishWS(path string) (*websocket.Conn, error) {
	prot := "ws"
	if c.Config.UseHTTPS {
		prot = "wss"
	}

	url := fmt.Sprintf("%s://%s:%d%s", prot, c.Config.APIHost, c.Config.APIPort, path)

	header := make(http.Header)
	header.Add(api.HeaderSessionToken, c.SessionToken)

	dialer := &websocket.Dialer{}
	conn, _, err := dialer.Dial(url, header)
	return conn, err
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

	if err := c.Request(RouteSessionInit, ReqPost, &req, &res); err != nil {
		return err
	}

	c.SessionToken = res.Token

	return nil
}

func (c *Client) PollLoggerSink(stop <-chan struct{}, sinkRoute string) (<-chan string, error) {
	path := fmt.Sprintf("%s", sinkRoute)

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(path)
	if err != nil {
		return nil, err
	}

	evChan := make(chan string)

	go func() {
		defer close(evChan)

		for {
			var ev string

			if err = ws.ReadJSON(&ev); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case evChan <- ev:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) PollAccounts(stop <-chan struct{}, accountID *string) (<-chan Account, error) {
	path := fmt.Sprintf("%s?", RouteWSAccounts)
	if accountID != nil {
		path = fmt.Sprintf("%sid=%s&", path, *accountID)
	}

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(path)
	if err != nil {
		return nil, err
	}

	evChan := make(chan Account)

	go func() {
		defer close(evChan)

		for {
			var ev Account

			if err = ws.ReadJSON(&ev); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case evChan <- ev:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) PollContracts(stop <-chan struct{}, contractID *string) (<-chan interface{}, error) {
	path := fmt.Sprintf("%s?", RouteWSContracts)
	if contractID != nil {
		path = fmt.Sprintf("%sid=%s&", path, *contractID)
	}

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(path)
	if err != nil {
		return nil, err
	}

	evChan := make(chan interface{})

	go func() {
		defer close(evChan)

		for {
			var ev interface{}

			if err = ws.ReadJSON(&ev); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case evChan <- ev:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) PollTransactions(stop <-chan struct{}, txID *string, senderID *string, creatorID *string) (<-chan Transaction, error) {
	path := fmt.Sprintf("%s?", RouteWSTransactions)
	if txID != nil {
		path = fmt.Sprintf("%stx_id=%s&", path, *txID)
	}
	if senderID != nil {
		path = fmt.Sprintf("%ssender=%s&", path, *senderID)
	}
	if creatorID != nil {
		path = fmt.Sprintf("%screator=%s&", path, *creatorID)
	}

	if stop == nil {
		stop = make(chan struct{})
	}

	ws, err := c.EstablishWS(path)
	if err != nil {
		return nil, err
	}

	evChan := make(chan Transaction)

	go func() {
		defer close(evChan)

		for {
			var ev Transaction

			if err = ws.ReadJSON(&ev); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case evChan <- ev:
			}
		}
	}()

	return evChan, nil
}

func (c *Client) GetLedgerStatus(senderID *string, creatorID *string, offset *uint64, limit *uint64) ([]Transaction, error) {
	var res []Transaction

	path := fmt.Sprintf("%s?", RouteLedger)
	if senderID != nil {
		path = fmt.Sprintf("%ssender=%s&", path, *senderID)
	}
	if creatorID != nil {
		path = fmt.Sprintf("%screator=%s&", path, *creatorID)
	}
	if offset != nil {
		path = fmt.Sprintf("%soffset=%d&", path, *offset)
	}
	if limit != nil {
		path = fmt.Sprintf("%slimit=%d&", path, *limit)
	}

	if err := c.Request(path, ReqGet, nil, &res); err != nil {
		return res, err
	}

	return res, nil
}

func (c *Client) GetAccount(accountID string) (Account, error) {
	var res Account

	path := fmt.Sprintf("%s/%s", RouteAccount, accountID)

	if err := c.Request(path, ReqGet, nil, &res); err != nil {
		return res, err
	}

	return res, nil
}

func (c *Client) GetContractCode(contractID string) (interface{}, error) {
	var res Transaction

	path := fmt.Sprintf("%s/%s", RouteContract, contractID)

	if err := c.Request(path, ReqGet, nil, &res); err != nil {
		return res, err
	}

	return res, nil
}

func (c *Client) GetContractPages(contractID string, index *uint64) (interface{}, error) {
	var res Transaction

	path := fmt.Sprintf("%s/%s/page", RouteContract, contractID)
	if index != nil {
		path = fmt.Sprintf("%s/%d", path, *index)
	}

	if err := c.Request(path, ReqGet, nil, &res); err != nil {
		return res, err
	}

	return res, nil
}

func (c *Client) ListTransactions(senderID *string, creatorID *string, offset *uint64, limit *uint64) ([]Transaction, error) {
	var res []Transaction

	path := fmt.Sprintf("%s?", RouteTxList)
	if senderID != nil {
		path = fmt.Sprintf("%ssender=%s&", path, *senderID)
	}
	if creatorID != nil {
		path = fmt.Sprintf("%screator=%s&", path, *creatorID)
	}
	if offset != nil {
		path = fmt.Sprintf("%soffset=%d&", path, *offset)
	}
	if limit != nil {
		path = fmt.Sprintf("%slimit=%d&", path, *limit)
	}

	if err := c.Request(path, ReqGet, nil, &res); err != nil {
		return res, err
	}

	return res, nil
}

func (c *Client) GetTransaction(txID string) (Transaction, error) {
	var res Transaction

	path := fmt.Sprintf("%s/%s", RouteTxList, txID)

	if err := c.Request(path, ReqGet, nil, &res); err != nil {
		return res, err
	}

	return res, nil
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

	if err := c.Request(RouteTxSend, ReqPost, &req, &res); err != nil {
		return res, err
	}

	return res, nil
}
