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

package api

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

func (g *Gateway) sendTransaction(ctx *fasthttp.RequestCtx) {
	req := new(sendTransactionRequest)

	if g.Ledger != nil && g.Ledger.TakeSendQuota() == false {
		g.renderError(ctx, ErrInternal(errors.New("rate limit")))
		return
	}

	parser := g.parserPool.Get()
	defer g.parserPool.Put(parser)

	err := req.bind(parser, ctx.PostBody())

	if err != nil {
		g.renderError(ctx, ErrBadRequest(err))
		return
	}

	tx := wavelet.NewSignedTransaction(
		req.sender, req.Nonce, req.Block,
		sys.Tag(req.Tag), req.payload, req.signature,
	)

	// TODO(kenta): check signature and nonce

	if err = g.Ledger.AddTransaction(tx); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error adding your transaction to graph")))
		return
	}

	g.render(ctx, &sendTransactionResponse{ledger: g.Ledger, tx: &tx})
}

func (g *Gateway) ledgerStatus(ctx *fasthttp.RequestCtx) {
	g.render(ctx, &ledgerStatusResponse{
		client:    g.Client,
		ledger:    g.Ledger,
		publicKey: g.Keys.PublicKey(),
	})
}

func (g *Gateway) listTransactions(ctx *fasthttp.RequestCtx) {
	// TODO

	//var sender wavelet.AccountID
	//var offset, limit uint64
	//var err error

	//queryArgs := ctx.QueryArgs()
	//if raw := string(queryArgs.Peek("sender")); len(raw) > 0 {
	//slice, err := hex.DecodeString(raw)
	//if err != nil {
	//g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "sender ID must be presented as valid hex")))
	//return
	//}

	//if len(slice) != wavelet.SizeAccountID {
	//g.renderError(ctx, ErrBadRequest(errors.Errorf("sender ID must be %d bytes long", wavelet.SizeAccountID)))
	//return
	//}

	//copy(sender[:], slice)
	//}

	//if raw := string(queryArgs.Peek("offset")); len(raw) > 0 {
	//offset, err = strconv.ParseUint(raw, 10, 64)

	//if err != nil {
	//g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "could not parse offset")))
	//return
	//}
	//}

	//if raw := string(queryArgs.Peek("limit")); len(raw) > 0 {
	//limit, err = strconv.ParseUint(raw, 10, 64)

	//if err != nil {
	//g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "could not parse limit")))
	//return
	//}
	//}

	//if limit > maxPaginationLimit {
	//limit = maxPaginationLimit
	//}

	var transactions transactionList
	//var rootDepth = g.Ledger.Graph().RootDepth()

	//for _, tx := range g.Ledger.Graph().ListTransactions(offset, limit, sender) {
	//status := "received"

	//if tx.Depth <= rootDepth {
	//status = "applied"
	//}

	//transactions = append(transactions, &transaction{tx: tx, status: status})
	//}

	g.render(ctx, transactions)
}

func (g *Gateway) getTransaction(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "transaction ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeTransactionID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("transaction ID must be %d bytes long", wavelet.SizeTransactionID)))
		return
	}

	var id wavelet.TransactionID
	copy(id[:], slice)

	tx := g.Ledger.Transactions().Find(id)
	if tx == nil {
		g.renderError(ctx, ErrNotFound(errors.Errorf("could not find transaction with ID %x", id)))
		return
	}

	// block := g.Ledger.Blocks().Latest()

	res := &transaction{tx: tx}

	// if tx.Depth <= rootDepth {
	// 	res.status = "applied"
	// } else {
	// 	res.status = "received"
	// }

	g.render(ctx, res)
}

func (g *Gateway) getAccount(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeAccountID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", wavelet.SizeAccountID)))
		return
	}

	var id wavelet.AccountID
	copy(id[:], slice)

	snapshot := g.Ledger.Snapshot()

	balance, _ := wavelet.ReadAccountBalance(snapshot, id)
	gasBalance, _ := wavelet.ReadAccountContractGasBalance(snapshot, id)
	stake, _ := wavelet.ReadAccountStake(snapshot, id)
	reward, _ := wavelet.ReadAccountReward(snapshot, id)
	nonce, _ := wavelet.ReadAccountNonce(snapshot, id)
	_, isContract := wavelet.ReadAccountContractCode(snapshot, id)
	numPages, _ := wavelet.ReadAccountContractNumPages(snapshot, id)

	g.render(ctx, &account{
		ledger:     g.Ledger,
		id:         id,
		balance:    balance,
		gasBalance: gasBalance,
		stake:      stake,
		reward:     reward,
		nonce:      nonce,
		isContract: isContract,
		numPages:   numPages,
	})
}

func (g *Gateway) getContractCode(ctx *fasthttp.RequestCtx) {
	id, ok := ctx.UserValue("contract_id").(wavelet.TransactionID)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a TransactionID")))
		return
	}

	code, available := wavelet.ReadAccountContractCode(g.Ledger.Snapshot(), id)

	if len(code) == 0 || !available {
		g.renderError(ctx, ErrNotFound(errors.Errorf("could not find contract with ID %x", id)))
		return
	}

	ctx.Response.Header.Set("Content-Disposition", "attachment; filename="+hex.EncodeToString(id[:])+".wasm")
	ctx.Response.Header.Set("Content-Type", "application/wasm")
	ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(code)))

	_, _ = io.Copy(ctx, bytes.NewReader(code))
}

func (g *Gateway) getContractPages(ctx *fasthttp.RequestCtx) {
	id, ok := ctx.UserValue("contract_id").(wavelet.TransactionID)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a TransactionID")))
		return
	}

	var idx uint64
	var err error

	rawIdx, ok := ctx.UserValue("index").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("could not cast index into string")))
		return
	}

	if len(rawIdx) != 0 {
		idx, err = strconv.ParseUint(rawIdx, 10, 64)

		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.New("could not parse page index")))
			return
		}
	}

	snapshot := g.Ledger.Snapshot()

	numPages, available := wavelet.ReadAccountContractNumPages(snapshot, id)

	if !available {
		g.renderError(ctx, ErrNotFound(errors.Errorf("could not find any pages for contract with ID %x", id)))
		return
	}

	if idx >= numPages {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("contract with ID %x only has %d pages, but you requested page %d", id, numPages, idx)))
		return
	}

	page, available := wavelet.ReadAccountContractPage(snapshot, id, idx)

	if len(page) == 0 || !available {
		_, _ = ctx.Write([]byte{})
		return
	}

	_, _ = ctx.Write(page)
}

func (g *Gateway) connect(ctx *fasthttp.RequestCtx) {
	parser := g.parserPool.Get()
	v, err := parser.ParseBytes(ctx.PostBody())
	g.parserPool.Put(parser)

	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "error parsing request body")))
		return
	}

	addressVal := v.Get("address")
	if addressVal == nil {
		g.renderError(ctx, ErrBadRequest(errors.New("address is missing")))
		return
	}

	address, err := addressVal.StringBytes()
	if err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error extracting address from payload")))
		return
	}

	if _, err := g.Client.Dial(string(address)); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error connecting to peer")))
		return
	}

	g.render(ctx, &msgResponse{msg: fmt.Sprintf("Successfully connected to %s", address)})
}

func (g *Gateway) disconnect(ctx *fasthttp.RequestCtx) {
	parser := g.parserPool.Get()
	v, err := parser.ParseBytes(ctx.PostBody())
	g.parserPool.Put(parser)

	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "error parsing request body")))
		return
	}

	addressVal := v.Get("address")
	if addressVal == nil {
		g.renderError(ctx, ErrBadRequest(errors.New("address is missing")))
		return
	}

	address, err := addressVal.StringBytes()
	if err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error extracting address from payload")))
		return
	}

	if err := g.Client.DisconnectByAddress(string(address)); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error disconnecting from peer")))
		return
	}

	g.render(ctx, &msgResponse{msg: fmt.Sprintf("Successfully disconnected from %s", address)})
}

func (g *Gateway) restart(ctx *fasthttp.RequestCtx) {
	if err := g.KV.Close(); err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "error closing storage")))
		return
	}

	body := ctx.PostBody()
	if len(body) != 0 {
		parser := g.parserPool.Get()
		v, err := parser.ParseBytes(body)
		g.parserPool.Put(parser)

		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "error parsing request body")))
			return
		}

		if v.GetBool("hard") {
			dbDir := g.KV.Dir()
			if len(dbDir) != 0 {
				if err := os.RemoveAll(dbDir); err != nil {
					g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "error deleting storage content")))
					return
				}
			}
		}
	}

	if err := g.Ledger.Restart(); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(err, "error restarting node")))
		return
	}
}

func (g *Gateway) getAccountNonce(ctx *fasthttp.RequestCtx) {
	param, ok := ctx.UserValue("id").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New("id must be a string")))
		return
	}

	slice, err := hex.DecodeString(param)
	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(err, "account ID must be presented as valid hex")))
		return
	}

	if len(slice) != wavelet.SizeAccountID {
		g.renderError(ctx, ErrBadRequest(errors.Errorf("account ID must be %d bytes long", wavelet.SizeAccountID)))
		return
	}

	var id wavelet.AccountID
	copy(id[:], slice)

	snapshot := g.Ledger.Snapshot()
	nonce, _ := wavelet.ReadAccountNonce(snapshot, id)
	block := g.Ledger.Blocks().Latest().Index

	g.render(ctx, &nonceResponse{Nonce: nonce, Block: block})
}
