package api

import (
	"encoding/hex"
	"fmt"
	"github.com/go-chi/render"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"net/http"
)

const (
	SessionInitMessage = "perlin_session_init_"
)

var (
	_ render.Binder   = (*SessionInitRequest)(nil)
	_ render.Renderer = (*SessionInitResponse)(nil)

	_ render.Binder   = (*SendTransactionRequest)(nil)
	_ render.Renderer = (*SendTransactionResponse)(nil)

	_ render.Renderer = (*LedgerStatusResponse)(nil)

	_ render.Renderer = (*Transaction)(nil)

	_ render.Renderer = (*Account)(nil)
)

type SessionInitRequest struct {
	PublicKey  string `json:"public_key"`
	Signature  string `json:"signature"`
	TimeMillis uint64 `json:"time_millis"`
}

func (s *SessionInitRequest) Bind(r *http.Request) error {
	publicKey, err := hex.DecodeString(s.PublicKey)
	if err != nil {
		return errors.Wrap(err, "public key provided is not hex-formatted")
	}

	if len(publicKey) != common.SizeAccountID {
		return errors.Errorf("public key must be size %d", common.SizeAccountID)
	}

	signature, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "signature provided is not hex-formatted")
	}

	if len(signature) != common.SizeSignature {
		return errors.Errorf("signature must be size %d", common.SizeSignature)
	}

	err = eddsa.Verify(publicKey, []byte(fmt.Sprintf("%s%d", SessionInitMessage, s.TimeMillis)), signature)
	if err != nil {
		return errors.Wrap(err, "signature verification failed")
	}

	return nil
}

type SessionInitResponse struct {
	Token string `json:"token"`
}

func (s *SessionInitResponse) Render(w http.ResponseWriter, r *http.Request) error {
	return nil
}

type SendTransactionRequest struct {
	Sender    string `json:"sender"`
	Tag       byte   `json:"tag"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`

	// Internal fields.
	creator   common.AccountID
	signature common.Signature
	payload   []byte
}

func (s *SendTransactionRequest) Bind(r *http.Request) error {
	sender, err := hex.DecodeString(s.Sender)
	if err != nil {
		return errors.Wrap(err, "sender public key provided is not hex-formatted")
	}

	if len(sender) != common.SizeAccountID {
		return errors.Errorf("sender public key must be size %d", common.SizeAccountID)
	}

	if s.Tag > sys.TagStake {
		return errors.New("unknown transaction tag specified")
	}

	s.payload, err = hex.DecodeString(s.Payload)
	if err != nil {
		return errors.Wrap(err, "payload provided is not hex-formatted")
	}

	signature, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "sender signature provided is not hex-formatted")
	}

	if len(signature) != common.SizeSignature {
		return errors.Errorf("sender signature must be size %d", common.SizeSignature)
	}

	err = eddsa.Verify(sender, append([]byte{s.Tag}, s.payload...), signature)
	if err != nil {
		return errors.Wrap(err, "sender signature verification failed")
	}

	copy(s.creator[:], sender)
	copy(s.signature[:], signature)

	return nil
}

type SendTransactionResponse struct {
	ID       string   `json:"tx_id"`
	Parents  []string `json:"parent_ids"`
	Critical bool     `json:"is_critical"`

	// Internal fields.
	ledger *wavelet.Ledger
	tx     *wavelet.Transaction
}

func (s *SendTransactionResponse) Render(w http.ResponseWriter, r *http.Request) error {
	if s.ledger == nil || s.tx == nil {
		return errors.New("insufficient parameters were provided")
	}

	s.ID = hex.EncodeToString(s.tx.ID[:])

	for _, parentID := range s.tx.ParentIDs {
		s.Parents = append(s.Parents, hex.EncodeToString(parentID[:]))
	}

	s.Critical = s.tx.IsCritical(s.ledger.Difficulty())

	return nil
}

type LedgerStatusResponse struct {
	PublicKey     string   `json:"public_key"`
	HostAddress   string   `json:"address"`
	PeerAddresses []string `json:"peers"`

	RootID     string `json:"root_id"`
	ViewID     uint64 `json:"view_id"`
	Difficulty uint64 `json:"difficulty"`

	// Internal fields.
	node   *noise.Node
	ledger *wavelet.Ledger
}

func (s *LedgerStatusResponse) Render(w http.ResponseWriter, r *http.Request) error {
	if s.node == nil || s.ledger == nil {
		return errors.New("insufficient parameters were provided")
	}

	s.PublicKey = hex.EncodeToString(s.node.Keys.PublicKey())
	s.HostAddress = s.node.ExternalAddress()
	s.PeerAddresses = skademlia.Table(s.node).GetPeers()

	s.RootID = hex.EncodeToString(s.ledger.Root().ID[:])
	s.ViewID = s.ledger.ViewID()
	s.Difficulty = s.ledger.Difficulty()

	return nil
}

type Transaction struct {
	ID string `json:"id"`

	Sender  string `json:"sender"`
	Creator string `json:"creator"`

	Parents []string `json:"parents"`

	Timestamp uint64 `json:"timestamp"`

	Tag     byte   `json:"tag"`
	Payload []byte `json:"payload"`

	AccountsMerkleRoot string `json:"accounts_root"`

	SenderSignature  string `json:"sender_signature"`
	CreatorSignature string `json:"creator_signature"`

	Depth uint64 `json:"depth"`

	// Internal fields.
	tx *wavelet.Transaction
}

func (s *Transaction) Render(w http.ResponseWriter, r *http.Request) error {
	if s.tx == nil {
		return errors.New("insufficient fields specified")
	}

	s.ID = hex.EncodeToString(s.tx.ID[:])
	s.Sender = hex.EncodeToString(s.tx.Sender[:])
	s.Creator = hex.EncodeToString(s.tx.Creator[:])
	s.Timestamp = s.tx.Timestamp
	s.Tag = s.tx.Tag
	s.Payload = s.tx.Payload
	s.AccountsMerkleRoot = hex.EncodeToString(s.tx.AccountsMerkleRoot[:])
	s.SenderSignature = hex.EncodeToString(s.tx.SenderSignature[:])
	s.CreatorSignature = hex.EncodeToString(s.tx.CreatorSignature[:])
	s.Depth = s.tx.Depth()

	for _, parentID := range s.tx.ParentIDs {
		s.Parents = append(s.Parents, hex.EncodeToString(parentID[:]))
	}

	return nil
}

type Account struct {
	PublicKey string `json:"public_key"`
	Balance   uint64 `json:"balance"`
	Stake     uint64 `json:"stake"`

	IsContract bool   `json:"is_contract"`
	NumPages   uint64 `json:"num_mem_pages,omitempty"`

	// Internal fields.
	id     common.AccountID
	ledger *wavelet.Ledger
}

func (s *Account) Render(w http.ResponseWriter, r *http.Request) error {
	if s.ledger == nil || s.id == common.ZeroAccountID {
		return errors.New("insufficient fields specified")
	}

	s.PublicKey = hex.EncodeToString(s.id[:])
	s.Balance, _ = s.ledger.Accounts.ReadAccountBalance(s.id)
	s.Stake, _ = s.ledger.Accounts.ReadAccountStake(s.id)
	s.NumPages, s.IsContract = s.ledger.Accounts.ReadAccountContractNumPages(s.id)

	return nil
}

type ErrResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	StatusText string `json:"status"`          // user-level status message
	AppCode    int64  `json:"code,omitempty"`  // application-specific error code
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrBadRequest(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
		StatusText:     "Bad request.",
		ErrorText:      err.Error(),
	}
}

func ErrInternal(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
		StatusText:     "Internal error.",
		ErrorText:      err.Error(),
	}
}
