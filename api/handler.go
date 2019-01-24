package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/events"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/security"
	"github.com/perlin-network/wavelet/stats"
	"github.com/pkg/errors"
	"gopkg.in/go-playground/validator.v9"
)

var (
	validate = validator.New()
)

func (s *service) pollAccountHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanAccessLedger {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	headers := make(http.Header)
	headers.Add(HeaderWebsocketProtocol, ctx.session.ID)

	conn, err := s.upgrader.Upgrade(ctx.response, ctx.request, headers)
	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "cannot upgrade connection")
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

	return http.StatusOK, nil, nil
}

func (s *service) pollTransactionHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanPollTransaction {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	eventType := ctx.request.URL.Query().Get("event")

	switch eventType {
	case "accepted":
	case "applied":
	default:
		return http.StatusBadRequest, nil, errors.New("event poll type not specified")
	}

	headers := make(http.Header)
	headers.Add(HeaderWebsocketProtocol, ctx.session.ID)

	conn, err := s.upgrader.Upgrade(ctx.response, ctx.request, headers)
	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "cannot upgrade connection")
	}

	closeSignal := make(chan struct{})

	report := func(txID string) bool {
		var tx *database.Transaction
		var err error

		s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
			tx, err = ledger.GetBySymbol(txID)
		})

		if err != nil {
			return true
		}

		if tx.Tag == params.TagCreateContract {
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

	return http.StatusOK, nil, nil
}

func (s *service) listTransactionHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanPollTransaction {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	var transactions []*database.Transaction
	var listParams ListTransactionsRequest

	if err := ctx.readJSON(&listParams); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if err := validate.Struct(listParams); err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid request")
	}

	s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
		// If paginate is blank, return the last 50 transactions.
		if listParams.Offset == nil || listParams.Limit == nil {
			total, limit := ledger.NumTransactions(), uint64(50)
			if limit > total {
				limit = total
			}

			offset := total - limit

			listParams.Limit = &limit
			listParams.Offset = &offset
		}
		transactions = ledger.PaginateTransactions(*listParams.Offset, *listParams.Limit)
	})

	for _, tx := range transactions {
		if tx.Tag == params.TagCreateContract {
			tx.Payload = []byte("<code placeholder>")
		}
	}

	return http.StatusOK, transactions, nil
}

func (s *service) executeContractHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanAccessLedger {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	var req ExecuteContractRequest
	if err := ctx.readJSON(&req); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if err := validate.Struct(req); err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid request")
	}

	var result []byte
	var err error

	s.wavelet.Ledger.Do(func(ledger *wavelet.Ledger) {
		result, err = ledger.ExecuteContract(req.ContractID, req.Entry, req.Param)
	})

	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "contract exited with error")
	} else {
		return http.StatusOK, &ExecuteContractResponse{
			Result: result,
		}, nil
	}
}

func (s *service) getContractHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanAccessLedger {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	var req GetContractRequest
	if err := ctx.readJSON(&req); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if err := validate.Struct(req); err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid request")
	}

	var contractCode []byte
	var err error

	s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
		contractCode, err = ledger.LoadContract(req.TransactionID)
	})

	if err != nil {
		return http.StatusBadRequest, nil, errors.Wrapf(err, "transaction %s does not exist", req.TransactionID)
	}

	res := &TransactionResponse{
		TransactionID: req.TransactionID,
		Code:          contractCode,
	}

	return http.StatusOK, res, nil
}

func (s *service) sendContractHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanSendTransaction {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	ctx.request.ParseMultipartForm(MaxContractUploadSize)

	file, header, err := ctx.request.FormFile(UploadFormField)

	if err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid format")
	}
	defer file.Close()

	if header.Size > MaxContractUploadSize {
		return http.StatusBadRequest, nil, errors.New("file too large")
	}

	var bb bytes.Buffer
	io.Copy(&bb, file)

	contract := &wavelet.Contract{
		Code: bb.Bytes(),
	}

	payload, err := json.Marshal(contract)
	if err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "Failed to marshal smart contract deployment payload.")
	}

	wired := s.wavelet.MakeTransaction(params.TagCreateContract, payload)
	go s.wavelet.BroadcastTransaction(wired)

	res := &TransactionResponse{
		TransactionID: graph.Symbol(wired),
	}

	return http.StatusOK, res, nil
}

func (s *service) listContractsHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanAccessLedger {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	var contracts []*wavelet.Contract
	var listParams ListContractsRequest
	var res []*TransactionResponse

	if err := ctx.readJSON(&listParams); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if err := validate.Struct(listParams); err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid request")
	}

	s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
		// If paginate is blank, return the last 50 contracts.
		if listParams.Offset == nil || listParams.Limit == nil {
			total, limit := ledger.NumContracts(), uint64(50)
			if limit > total {
				limit = total
			}

			offset := total - limit

			listParams.Limit = &limit
			listParams.Offset = &offset
		}
		contracts = ledger.PaginateContracts(*listParams.Offset, *listParams.Limit)
	})

	for _, contract := range contracts {
		res = append(res, &TransactionResponse{
			TransactionID: contract.TransactionID,
		})
	}

	return http.StatusOK, res, nil
}

func (s *service) ledgerStateHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanAccessLedger {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	state := &LedgerState{}

	if s.network != nil {
		plugin, exists := s.network.Plugin(discovery.PluginID)
		if !exists {
			return http.StatusInternalServerError, nil, errors.New("peer discovery disabled")
		}

		routes := plugin.(*discovery.Plugin).Routes

		state.PublicKey = s.network.ID.PublicKeyHex()
		state.Address = s.network.ID.Address
		state.Peers = routes.GetPeerAddresses()
	}

	s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
		state.State = ledger.Snapshot()
	})

	return http.StatusOK, state, nil
}

func (s *service) sendTransactionHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanSendTransaction {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	var req SendTransactionRequest

	if err := ctx.readJSON(&req); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if err := validate.Struct(req); err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid request")
	}

	wired := s.wavelet.MakeTransaction(req.Tag, req.Payload)
	go s.wavelet.BroadcastTransaction(wired)

	res := &TransactionResponse{
		TransactionID: graph.Symbol(wired),
	}

	return http.StatusOK, res, nil
}

func (s *service) getTransactionHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	var symbol string

	if err := ctx.readJSON(&symbol); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if len(symbol) != 64 {
		return http.StatusBadRequest, nil, errors.New("transaction symbol ID must be a length-64 hex-encoded string")
	}

	_, err := hex.DecodeString(symbol)

	if err != nil {
		return http.StatusBadRequest, nil, errors.New("transaction symbol ID must be a length-64 hex-encoded string")
	}

	var tx *database.Transaction

	s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
		tx, err = ledger.GetBySymbol(symbol)
	})

	if err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "could not find transaction given symbol ID")
	}

	return http.StatusOK, tx, nil
}

func (s *service) resetStatsHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanControlStats {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	stats.Reset()

	return http.StatusOK, "OK", nil
}

func (s *service) getAccountHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	if !ctx.session.Permissions.CanAccessLedger {
		return http.StatusForbidden, nil, errors.New("permission denied")
	}

	var encodedAccountID string
	if err := ctx.readJSON(&encodedAccountID); err != nil {
		return http.StatusBadRequest, nil, err
	}

	accountID, err := hex.DecodeString(encodedAccountID)
	if err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "failed to hex-decode account id")
	}

	var account *wavelet.Account

	s.wavelet.LedgerDo(func(ledger wavelet.LedgerInterface) {
		account = wavelet.NewAccount(ledger.(*wavelet.Ledger), accountID)
	})

	if err != nil {
		return http.StatusOK, make(map[string][]byte), nil
	}

	state := make(map[string][]byte)

	account.Range(func(key string, value []byte) {
		state[key] = value
	})

	return http.StatusOK, state, nil
}

func (s *service) serverVersionHandler(ctx *requestContext) (int, interface{}, error) {
	if err := ctx.loadSession(); err != nil {
		return http.StatusForbidden, nil, err
	}

	res := &ServerVersion{
		Version:   params.Version,
		GitCommit: params.GitCommit,
		OSArch:    params.OSArch,
	}

	return http.StatusOK, res, nil
}

// sessionInitHandler initialize a session.
func (s *service) sessionInitHandler(ctx *requestContext) (int, interface{}, error) {
	var credentials CredentialsRequest
	if err := ctx.readJSON(&credentials); err != nil {
		return http.StatusBadRequest, nil, err
	}

	if err := validate.Struct(credentials); err != nil {
		return http.StatusBadRequest, nil, errors.Wrap(err, "invalid credentials")
	}

	info, ok := s.clients[credentials.PublicKey]
	if !ok {
		return http.StatusForbidden, nil, errors.New("invalid token")
	}

	// TODO: this check doesn't work if the client clock is off from the server's clock,
	//  but we need something to prevent reused credentials
	/*
		timeOffset := credentials.TimeMillis - time.Now().UnixNano()/int64(time.Millisecond)
		if timeOffset < 0 {
			timeOffset = -timeOffset
		}

		if timeOffset > MaxTimeOffsetInMs {
			return http.StatusForbidden, nil, errors.New("token expired")
		}
	*/

	rawSignature, err := hex.DecodeString(credentials.Sig)
	if err != nil {
		return http.StatusForbidden, nil, errors.New("invalid signature")
	}

	rawPublicKey, err := hex.DecodeString(credentials.PublicKey)
	if err != nil {
		return http.StatusForbidden, nil, errors.New("invalid public key")
	}

	expected := fmt.Sprintf("%s%d", SessionInitSigningPrefix, credentials.TimeMillis)
	if !security.Verify(rawPublicKey, []byte(expected), rawSignature) {
		return http.StatusForbidden, nil, errors.New("signature verification failed")
	}

	session, err := s.registry.newSession(info.Permissions)
	if err != nil {
		return http.StatusForbidden, nil, errors.New("sessions limited")
	}

	return http.StatusOK, SessionResponse{Token: session.ID}, nil
}
