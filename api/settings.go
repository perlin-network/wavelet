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
)

// Options represents available options for a local user.
type Options struct {
	ListenAddr string
	Clients    []*ClientInfo
}

// ClientInfo represents a single clients info.
type ClientInfo struct {
	PublicKey   string
	Permissions ClientPermissions
}

// ClientPermissions represents a single client permissions.
type ClientPermissions struct {
	CanPollTransaction bool
	CanSendTransaction bool
	CanControlStats    bool
	CanAccessLedger    bool
}
