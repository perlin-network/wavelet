package api

// Constants used in the api package
const (
	RouteSessionInit = "/session/init"

	RouteLedgerState = "/ledger/state"

	RouteTransactionList = "/transaction/list"
	RouteTransactionPoll = "/transaction/poll"
	RouteTransactionSend = "/transaction/send"
	RouteTransaction     = "/transaction"

	RouteContractSend = "/contract/send"
	RouteContractGet  = "/contract/get"
	RouteContractList = "/contract/list"

	RouteStatsReset = "/stats/reset"

	RouteAccountGet  = "/account/get"
	RouteAccountPoll = "/account/poll"

	RouteServerVersion = "/server/version"

	HeaderSessionToken      = "X-Session-Token"
	HeaderWebsocketProtocol = "Sec-Websocket-Protocol"
	HeaderUserAgent         = "User-Agent"

	MaxAllowableSessions     = 50000
	MaxRequestBodySize       = 4 * 1024 * 1024
	MaxContractUploadSize    = 4 * 1024 * 1024
	MaxSessionTimeoutMinutes = 5
	MaxTimeOffsetInMs        = 5000

	SessionInitSigningPrefix = "perlin_session_init_"
	UploadFormField          = "uploadFile"
)
