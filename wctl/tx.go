package wctl

import (
	"encoding/binary"
	"encoding/hex"
	"net/url"
	"strconv"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/wavelet/api"
	"github.com/pkg/errors"
)

var (
	// ErrInsufficientPerls is returned when you don't have enough PERLs.
	ErrInsufficientPerls = errors.New("Insufficient PERLs")
)

// ListTransactions calls the /tx endpoint of the API to list all transactions.
// The arguments are optional, zero values would default them.
func (c *Client) ListTransactions(senderID string, creatorID string,
	offset uint64, limit uint64) (api.TransactionList, error) {

	vals := url.Values{}

	if senderID != "" {
		vals.Set("sender", string(senderID[:]))
	}

	if creatorID != "" {
		vals.Set("creator", string(creatorID[:]))
	}

	if offset != 0 {
		vals.Set("offset", strconv.FormatUint(offset, 10))
	}

	if limit != 0 {
		vals.Set("limit", strconv.FormatUint(limit, 10))
	}

	path := RouteTxList + "?" + vals.Encode()

	var res api.TransactionList
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return res, nil
}

// GetTransaction calls the /tx endpoint to query a single transaction.
func (c *Client) GetTransaction(txID [32]byte) (*api.Transaction, error) {
	path := RouteTxList + "/" + hex.EncodeToString(txID[:])

	var res api.Transaction
	if err := c.RequestJSON(path, ReqGet, nil, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// SendTransaction calls the /tx/send endpoint to send a raw payload.
// Payloads are best crafted with wavelet.Transfer.
func (c *Client) SendTransaction(tag byte, payload []byte) (*api.TxResponse, error) {
	var res api.TxResponse

	nonce := c.Nonce.Add(1)
	block := c.Block.Load()

	var nonceBuf [8]byte
	binary.BigEndian.PutUint64(nonceBuf[:], nonce)

	var blockBuf [8]byte
	binary.BigEndian.PutUint64(blockBuf[:], block)

	signature := edwards25519.Sign(
		c.PrivateKey,
		append(nonceBuf[:], append(blockBuf[:], append([]byte{tag}, payload...)...)...),
	)

	req := api.TxRequest{
		Sender:    c.PublicKey,
		Nonce:     nonce,
		Block:     block,
		Tag:       tag,
		Payload:   payload,
		Signature: signature,
	}

	if err := c.RequestJSON(RouteTxSend, ReqPost, &req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// SendTransfer sends a wavelet.Transfer instead of a Payload.
func (c *Client) sendTransfer(tag byte, transfer Marshalable) (*api.TxResponse, error) {
	b, err := transfer.Marshal()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal")
	}

	return c.SendTransaction(tag, b)
}
