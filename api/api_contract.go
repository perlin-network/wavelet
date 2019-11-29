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
	"io"
	"strconv"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

func (g *Gateway) getContractCode(ctx *fasthttp.RequestCtx) {
	id, ok := ctx.UserValue("contract_id").(wavelet.TransactionID)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New(
			"id must be a TransactionID")))
		return
	}

	code, available := wavelet.ReadAccountContractCode(g.Ledger.Snapshot(), id)

	if len(code) == 0 || !available {
		g.renderError(ctx, ErrNotFound(errors.Errorf(
			"could not find contract with ID %x", id)))
		return
	}

	ctx.Response.Header.Set("Content-Disposition",
		"attachment; filename="+hex.EncodeToString(id[:])+".wasm")
	ctx.Response.Header.Set("Content-Type", "application/wasm")
	ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(code)))

	_, _ = io.Copy(ctx, bytes.NewReader(code))
}

func (g *Gateway) getContractPages(ctx *fasthttp.RequestCtx) {
	id, ok := ctx.UserValue("contract_id").(wavelet.TransactionID)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New(
			"id must be a TransactionID")))
		return
	}

	var idx uint64
	var err error

	rawIdx, ok := ctx.UserValue("index").(string)
	if !ok {
		g.renderError(ctx, ErrBadRequest(errors.New(
			"could not cast index into string")))
		return
	}

	if len(rawIdx) != 0 {
		idx, err = strconv.ParseUint(rawIdx, 10, 64)

		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.New(
				"could not parse page index")))
			return
		}
	}

	snapshot := g.Ledger.Snapshot()

	numPages, available := wavelet.ReadAccountContractNumPages(snapshot, id)

	if !available {
		g.renderError(ctx, ErrNotFound(errors.Errorf(
			"could not find any pages for contract with ID %x", id)))
		return
	}

	if idx >= numPages {
		g.renderError(ctx, ErrBadRequest(errors.Errorf(
			"contract with ID %x only has %d pages, but you requested page %d",
			id, numPages, idx)))
		return
	}

	page, available := wavelet.ReadAccountContractPage(snapshot, id, idx)

	if len(page) == 0 || !available {
		_, _ = ctx.Write([]byte{})
		return
	}

	_, _ = ctx.Write(page)
}
