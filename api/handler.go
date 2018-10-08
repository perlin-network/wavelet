package api

import (
	"encoding/hex"
	"net/http"
	"time"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/wavelet"
	cmdUtils "github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/events"
	"github.com/perlin-network/wavelet/stats"
)

func (s *service) pollAccountHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	headers := make(http.Header)
	headers.Add("Sec-Websocket-Protocol", ctx.session.ID)

	conn, err := s.upgrader.Upgrade(ctx.response, ctx.request, headers)
	if err != nil {
		panic(err)
	}

	closeSignal := make(chan struct{})

	events.Subscribe(nil, func(ev *events.AccountUpdateEvent) bool {
		if err := conn.WriteJSON(ev); err != nil {
			close(closeSignal)
			return false
		}
		return true
	})

	<-closeSignal
}

func (s *service) listTransactionHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	var paginate struct {
		Offset *uint64 `json:"offset"`
		Limit  *uint64 `json:"limit"`
	}

	var transactions []*database.Transaction

	err := ctx.readJSON(&paginate)

	s.wavelet.Ledger.Do(func(ledger *wavelet.Ledger) {
		// If there are errors in reading the JSON, return the last 50 transactions.
		if err != nil || (paginate.Offset == nil || paginate.Limit == nil) {
			total, limit := ledger.NumTransactions(), uint64(50)
			if limit > total {
				limit = total
			}

			offset := total - limit

			paginate.Limit = &limit
			paginate.Offset = &offset
		}

		transactions = ledger.PaginateTransactions(*paginate.Offset, *paginate.Limit)
	})

	for _, tx := range transactions {
		if tx.Tag == "create_contract" {
			tx.Payload = []byte("<code here>")
		}
	}

	ctx.WriteJSON(http.StatusOK, transactions)
}

func (s *service) pollTransactionHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	if !ctx.session.Permissions.CanPollTransaction {
		ctx.WriteJSON(http.StatusForbidden, "cannot poll transaction")
		return
	}

	eventType := ctx.request.URL.Query().Get("event")

	switch eventType {
	case "accepted":
	case "applied":
	default:
		ctx.WriteJSON(http.StatusBadRequest, "event poll type not specified")
		return
	}

	headers := make(http.Header)
	headers.Add("Sec-Websocket-Protocol", ctx.session.ID)

	conn, err := s.upgrader.Upgrade(ctx.response, ctx.request, headers)
	if err != nil {
		panic(err)
	}

	closeSignal := make(chan struct{})

	report := func(txID string) bool {
		var tx *database.Transaction
		var err error

		s.wavelet.Ledger.Do(func(ledger *wavelet.Ledger) {
			tx, err = ledger.GetBySymbol(txID)
		})

		if err != nil {
			return true
		}

		if tx.Tag == "create_contract" {
			tx.Payload = []byte("<code here>")
		}

		if err := conn.WriteJSON(tx); err != nil {
			close(closeSignal)
			return false
		}
		return true
	}

	switch eventType {
	case "applied":
		events.Subscribe(nil, func(ev *events.TransactionAppliedEvent) bool {
			return report(ev.ID)
		})
	case "accepted":
		events.Subscribe(nil, func(ev *events.TransactionAcceptedEvent) bool {
			return report(ev.ID)
		})
	}

	<-closeSignal
}

func (s *service) ledgerStateHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	plugin, exists := s.network.Plugin(discovery.PluginID)

	if !exists {
		ctx.WriteJSON(http.StatusInternalServerError, "ledger does not have peer discovery enabled")
		return
	}

	routes := plugin.(*discovery.Plugin).Routes

	state := struct {
		PublicKey string                 `json:"public_key"`
		Address   string                 `json:"address"`
		Peers     []string               `json:"peers"`
		State     map[string]interface{} `json:"state"`
	}{
		PublicKey: s.network.ID.PublicKeyHex(),
		Address:   s.network.ID.Address,
		Peers:     routes.GetPeerAddresses(),
	}

	s.wavelet.Ledger.Do(func(ledger *wavelet.Ledger) {
		state.State = ledger.Snapshot()
	})

	ctx.WriteJSON(http.StatusOK, state)
}

func (s *service) sendTransactionHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	if !ctx.session.Permissions.CanSendTransaction {
		ctx.WriteJSON(http.StatusForbidden, "cannot send transaction")
		return
	}

	var info struct {
		Tag     string `json:"tag"`
		Payload []byte `json:"payload"`
	}

	if err := ctx.readJSON(&info); err != nil {
		return
	}

	wired := s.wavelet.MakeTransaction(info.Tag, info.Payload)
	go s.wavelet.BroadcastTransaction(wired)

	ctx.WriteJSON(http.StatusOK, "OK")
}

func (s *service) resetStatsHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	if !ctx.session.Permissions.CanControlStats {
		ctx.WriteJSON(http.StatusForbidden, "no stats permissions")
		return
	}

	stats.Reset()

	ctx.WriteJSON(http.StatusOK, "OK")
}

func (s *service) loadAccountHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	if !ctx.session.Permissions.CanPollTransaction {
		ctx.WriteJSON(http.StatusForbidden, "permission denied")
		return
	}

	var encodedAccountID string
	if err := ctx.readJSON(&encodedAccountID); err != nil {
		ctx.WriteJSON(http.StatusBadRequest, "missing accountID parameter")
		return
	}

	accountID, err := hex.DecodeString(encodedAccountID)
	if err != nil {
		ctx.WriteJSON(http.StatusBadRequest, "failed to hex-decode accountID")
		return
	}

	var account *wavelet.Account

	s.wavelet.Ledger.Do(func(ledger *wavelet.Ledger) {
		account, err = ledger.LoadAccount(accountID)
	})

	if err != nil {
		ctx.WriteJSON(http.StatusOK, make(map[string][]byte))
		return
	}

	info := make(map[string][]byte)

	account.Range(func(key string, value []byte) {
		info[key] = value
	})

	ctx.WriteJSON(http.StatusOK, info)
}

func (s *service) serverVersionHandler(ctx *requestContext) {
	if !ctx.loadSession() {
		return
	}

	info := &ServerVersion{
		Version:   cmdUtils.Version,
		GitCommit: cmdUtils.GitCommit,
	}
	ctx.WriteJSON(http.StatusOK, info)
}

// sessionInitHandler initialize a session.
func (s *service) sessionInitHandler(ctx *requestContext) {
	var credentials credentials
	if err := ctx.readJSON(&credentials); err != nil {
		return
	}

	var info *ClientInfo
	for _, clientInfo := range s.clients {
		if credentials.validate(clientInfo.AuthKey) == nil {
			info = clientInfo
			break
		}
	}

	if info == nil {
		ctx.WriteJSON(http.StatusForbidden, "invalid token")
		return
	}

	timeOffset := credentials.TimeMillis - time.Now().UnixNano()/int64(time.Millisecond)
	if timeOffset < 0 {
		timeOffset = -timeOffset
	}

	if timeOffset > 5000 {
		ctx.WriteJSON(http.StatusForbidden, "token too old")
		return
	}

	session := s.registry.newSession(info.Permissions)

	ctx.WriteJSON(http.StatusOK, SessionResponse{
		Token: session.ID,
	})
}
