package api

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

func (g *Gateway) connect(ctx *fasthttp.RequestCtx) {
	parser := g.parserPool.Get()
	v, err := parser.ParseBytes(ctx.PostBody())
	g.parserPool.Put(parser)

	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(
			err, "error parsing request body")))
		return
	}

	addressVal := v.Get("address")
	if addressVal == nil {
		g.renderError(ctx, ErrBadRequest(errors.New("address is missing")))
		return
	}

	address, err := addressVal.StringBytes()
	if err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(
			err, "error extracting address from payload")))
		return
	}

	if _, err := g.Client.Dial(string(address)); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(
			err, "error connecting to peer")))
		return
	}

	g.render(ctx, &MsgResponse{
		Message: fmt.Sprintf("Successfully connected to %s", address),
	})
}

func (g *Gateway) disconnect(ctx *fasthttp.RequestCtx) {
	parser := g.parserPool.Get()
	v, err := parser.ParseBytes(ctx.PostBody())
	g.parserPool.Put(parser)

	if err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(
			err, "error parsing request body")))
		return
	}

	addressVal := v.Get("address")
	if addressVal == nil {
		g.renderError(ctx, ErrBadRequest(errors.New("address is missing")))
		return
	}

	address, err := addressVal.StringBytes()
	if err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(
			err, "error extracting address from payload")))
		return
	}

	if err := g.Client.DisconnectByAddress(string(address)); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(
			err, "error disconnecting from peer")))
		return
	}

	g.render(ctx, &MsgResponse{
		Message: fmt.Sprintf("Successfully disconnected from %s", address),
	})
}

func (g *Gateway) restart(ctx *fasthttp.RequestCtx) {
	if err := g.KV.Close(); err != nil {
		g.renderError(ctx, ErrBadRequest(errors.Wrap(
			err, "error closing storage")))
		return
	}

	body := ctx.PostBody()
	if len(body) != 0 {
		parser := g.parserPool.Get()
		v, err := parser.ParseBytes(body)
		g.parserPool.Put(parser)

		if err != nil {
			g.renderError(ctx, ErrBadRequest(errors.Wrap(
				err, "error parsing request body")))
			return
		}

		if v.GetBool("hard") {
			dbDir := g.KV.Dir()
			if len(dbDir) != 0 {
				if err := os.RemoveAll(dbDir); err != nil {
					g.renderError(ctx, ErrBadRequest(errors.Wrap(
						err, "error deleting storage content")))
					return
				}
			}
		}
	}

	if err := g.Ledger.Restart(); err != nil {
		g.renderError(ctx, ErrInternal(errors.Wrap(
			err, "error restarting node")))
		return
	}
}
