package api

const (
	RouteSessionInit     = "/session/init"
	RouteLedgerState     = "/ledger/state"
	RouteTransactionList = "/transaction/list"
	RouteTransactionPoll = "/transaction/poll"
	RouteTransactionSend = "/transaction/send"
	RouteStatsReset      = "/stats/reset"
	RouteAccountLoad     = "/account/load"
	RouteAccountPoll     = "/account/poll"
	RouteServerVersion   = "/server/version"

	HeaderSessionToken      = "X-Session-Token"
	HeaderWebsocketProtocol = "Sec-Websocket-Protocol"
	HeaderUserAgent         = "User-Agent"

	MaxAllowableSessions = 50000
)
