package api

import (
	"encoding/hex"
	"fmt"
	"github.com/go-chi/render"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
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

	if len(publicKey) != wavelet.PublicKeySize {
		return errors.Errorf("public key must be size %d", wavelet.PublicKeySize)
	}

	signature, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "signature provided is not hex-formatted")
	}

	if len(signature) != wavelet.SignatureSize {
		return errors.Errorf("signature must be size %d", wavelet.SignatureSize)
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
	Payload   []byte `json:"payload"`
	Signature string `json:"signature"`

	// Internal fields.
	creator   [wavelet.PublicKeySize]byte
	signature [wavelet.SignatureSize]byte
}

func (s *SendTransactionRequest) Bind(r *http.Request) error {
	sender, err := hex.DecodeString(s.Sender)
	if err != nil {
		return errors.Wrap(err, "sender public key provided is not hex-formatted")
	}

	if len(sender) != wavelet.PublicKeySize {
		return errors.Errorf("sender public key must be size %d", wavelet.PublicKeySize)
	}

	if s.Tag > sys.TagStake {
		return errors.New("unknown transaction tag specified")
	}

	signature, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "sender signature provided is not hex-formatted")
	}

	if len(signature) != wavelet.SignatureSize {
		return errors.Errorf("sender signature must be size %d", wavelet.SignatureSize)
	}

	err = eddsa.Verify(sender, append([]byte{s.Tag}, s.Payload...), signature)
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
