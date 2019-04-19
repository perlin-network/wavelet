package api

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/_old"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	"net/http"
	"strconv"
)

const (
	SessionInitMessage = "perlin_session_init_"
)

type marshalableJSON interface {
	marshalJSON(arena *fastjson.Arena) ([]byte, error)
}

var (
	_ marshalableJSON = (*sessionInitResponse)(nil)

	_ marshalableJSON = (*sendTransactionResponse)(nil)

	_ marshalableJSON = (*ledgerStatusResponse)(nil)

	_ marshalableJSON = (*transaction)(nil)

	_ marshalableJSON = (*account)(nil)
)

type sessionInitRequest struct {
	PublicKey  string `json:"public_key"`
	Signature  string `json:"signature"`
	TimeMillis uint64 `json:"time_millis"`
}

func (s *sessionInitRequest) bind(parser *fastjson.Parser, body []byte) error {
	v, err := parser.ParseBytes(body)
	if err != nil {
		return err
	}

	s.PublicKey = string(v.GetStringBytes("public_key"))
	s.Signature = string(v.GetStringBytes("signature"))
	s.TimeMillis = v.GetUint64("time_millis")

	publicKeyBuf, err := hex.DecodeString(s.PublicKey)
	if err != nil {
		return errors.Wrap(err, "public key provided is not hex-formatted")
	}

	if len(publicKeyBuf) != common.SizeAccountID {
		return errors.Errorf("public key must be size %d", common.SizeAccountID)
	}

	signatureBuf, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "signature provided is not hex-formatted")
	}

	if len(signatureBuf) != common.SizeSignature {
		return errors.Errorf("signature must be size %d", common.SizeSignature)
	}

	var publicKey edwards25519.PublicKey
	copy(publicKey[:], publicKeyBuf)

	var signature edwards25519.Signature
	copy(signature[:], signatureBuf)

	if !edwards25519.Verify(publicKey, []byte(fmt.Sprintf("%s%d", SessionInitMessage, s.TimeMillis)), signature) {
		return errors.Wrap(err, "signature verification failed")
	}

	return nil
}

type sessionInitResponse struct {
	Token string `json:"token"`
}

func (s *sessionInitResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	o.Set("token", arena.NewString(s.Token))

	return o.MarshalTo(nil), nil
}

type sendTransactionRequest struct {
	Sender    string `json:"sender"`
	Tag       byte   `json:"tag"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`

	// Internal fields.
	creator   common.AccountID
	signature common.Signature
	payload   []byte
}

func (s *sendTransactionRequest) bind(parser *fastjson.Parser, body []byte) error {
	v, err := parser.ParseBytes(body)
	if err != nil {
		return err
	}

	s.Sender = string(v.GetStringBytes("sender"))
	s.Tag = byte(v.GetUint("tag"))
	s.Payload = string(v.GetStringBytes("payload"))
	s.Signature = string(v.GetStringBytes("signature"))

	senderBuf, err := hex.DecodeString(s.Sender)
	if err != nil {
		return errors.Wrap(err, "sender public key provided is not hex-formatted")
	}

	if len(senderBuf) != common.SizeAccountID {
		return errors.Errorf("sender public key must be size %d", common.SizeAccountID)
	}

	if s.Tag > sys.TagStake {
		return errors.New("unknown transaction tag specified")
	}

	s.payload, err = hex.DecodeString(s.Payload)
	if err != nil {
		return errors.Wrap(err, "payload provided is not hex-formatted")
	}

	signatureBuf, err := hex.DecodeString(s.Signature)
	if err != nil {
		return errors.Wrap(err, "sender signature provided is not hex-formatted")
	}

	if len(signatureBuf) != common.SizeSignature {
		return errors.Errorf("sender signature must be size %d", common.SizeSignature)
	}

	var sender edwards25519.PublicKey
	copy(sender[:], senderBuf)

	var signature edwards25519.Signature
	copy(signature[:], signatureBuf)

	if !edwards25519.Verify(sender, append([]byte{s.Tag}, s.payload...), signature) {
		return errors.Wrap(err, "sender signature verification failed")
	}

	copy(s.creator[:], senderBuf)
	copy(s.signature[:], signatureBuf)

	return nil
}

type sendTransactionResponse struct {
	// Internal fields.
	ledger *_old.Ledger
	tx     *_old.Transaction
}

func (s *sendTransactionResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.tx == nil {
		return nil, errors.New("insufficient parameters were provided")
	}

	o := arena.NewObject()

	o.Set("tx_id", arena.NewString(hex.EncodeToString(s.tx.ID[:])))

	if s.tx.ParentIDs != nil {
		parents := arena.NewArray()
		for i, parentID := range s.tx.ParentIDs {
			parents.SetArrayItem(i, arena.NewString(hex.EncodeToString(parentID[:])))
		}
		o.Set("parent_ids", parents)
	} else {
		o.Set("parent_ids", nil)
	}

	if s.tx.IsCritical(s.ledger.Difficulty()) {
		o.Set("is_critical", arena.NewTrue())
	} else {
		o.Set("is_critical", arena.NewFalse())
	}

	return o.MarshalTo(nil), nil
}

type ledgerStatusResponse struct {
	// Internal fields.

	node      *noise.Node
	ledger    *_old.Ledger
	network   *skademlia.Protocol
	publicKey edwards25519.PublicKey
}

func (s *ledgerStatusResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.node == nil || s.ledger == nil {
		return nil, errors.New("insufficient parameters were provided")
	}

	o := arena.NewObject()

	o.Set("public_key", arena.NewString(hex.EncodeToString(s.publicKey[:])))
	o.Set("address", arena.NewString(s.node.Addr().String()))
	o.Set("root_id", arena.NewString(hex.EncodeToString(s.ledger.Root().ID[:])))
	o.Set("view_id", arena.NewNumberString(strconv.FormatUint(s.ledger.ViewID(), 10)))
	o.Set("difficulty", arena.NewNumberString(strconv.FormatUint(s.ledger.Difficulty(), 10)))

	peers := s.network.Peers(s.node)
	if len(peers) > 0 {
		peersArray := arena.NewArray()
		for i := range peers {
			peersArray.SetArrayItem(i, arena.NewString(peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID).Address()))
		}
		o.Set("peers", peersArray)
	} else {
		o.Set("peers", nil)
	}

	return o.MarshalTo(nil), nil
}

type transaction struct {
	// Internal fields.
	tx *_old.Transaction
}

func (s *transaction) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o, err := s.getObject(arena)
	if err != nil {
		return nil, err
	}

	return o.MarshalTo(nil), nil
}

func (s *transaction) getObject(arena *fastjson.Arena) (*fastjson.Value, error) {
	if s.tx == nil {
		return nil, errors.New("insufficient fields specified")
	}

	o := arena.NewObject()

	o.Set("id", arena.NewString(hex.EncodeToString(s.tx.ID[:])))
	o.Set("sender", arena.NewString(hex.EncodeToString(s.tx.Sender[:])))
	o.Set("creator", arena.NewString(hex.EncodeToString(s.tx.Creator[:])))
	o.Set("timestamp", arena.NewNumberString(strconv.FormatUint(s.tx.Timestamp, 10)))
	o.Set("tag", arena.NewNumberInt(int(s.tx.Tag)))
	o.Set("payload", arena.NewString(base64.StdEncoding.EncodeToString(s.tx.Payload)))
	o.Set("accounts_root", arena.NewString(hex.EncodeToString(s.tx.AccountsMerkleRoot[:])))
	o.Set("sender_signature", arena.NewString(hex.EncodeToString(s.tx.SenderSignature[:])))
	o.Set("creator_signature", arena.NewString(hex.EncodeToString(s.tx.CreatorSignature[:])))
	o.Set("depth", arena.NewNumberString(strconv.FormatUint(s.tx.Depth(), 10)))

	if s.tx.ParentIDs != nil {
		parents := arena.NewArray()
		for i := range s.tx.ParentIDs {
			parents.SetArrayItem(i, arena.NewString(hex.EncodeToString(s.tx.ParentIDs[i][:])))
		}
		o.Set("parents", parents)
	} else {
		o.Set("parents", nil)
	}

	return o, nil
}

type transactionList []*transaction

func (s transactionList) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	list := arena.NewArray()

	for i, v := range s {
		o, err := v.getObject(arena)
		if err != nil {
			return nil, err
		}

		list.SetArrayItem(i, o)
	}

	return list.MarshalTo(nil), nil
}

type account struct {
	// Internal fields.
	id     common.AccountID
	ledger *_old.Ledger
}

func (s *account) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	if s.ledger == nil || s.id == common.ZeroAccountID {
		return nil, errors.New("insufficient fields specified")
	}

	snapshot := s.ledger.Snapshot()

	o := arena.NewObject()

	o.Set("public_key", arena.NewString(hex.EncodeToString(s.id[:])))

	balance, _ := _old.ReadAccountBalance(snapshot, s.id)
	o.Set("balance", arena.NewNumberString(strconv.FormatUint(balance, 10)))

	stake, _ := _old.ReadAccountStake(snapshot, s.id)
	o.Set("stake", arena.NewNumberString(strconv.FormatUint(stake, 10)))

	_, isContract := _old.ReadAccountContractCode(snapshot, s.id)
	if isContract {
		o.Set("is_contract", arena.NewTrue())
	} else {
		o.Set("is_contract", arena.NewFalse())
	}

	numPages, _ := _old.ReadAccountContractNumPages(snapshot, s.id)
	if numPages != 0 {
		o.Set("num_mem_pages", arena.NewNumberString(strconv.FormatUint(numPages, 10)))
	}

	return o.MarshalTo(nil), nil
}

type errResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code
}

func (e *errResponse) marshalJSON(arena *fastjson.Arena) ([]byte, error) {
	o := arena.NewObject()

	o.Set("status", arena.NewString("Bad request."))
	o.Set("error", arena.NewString(e.Err.Error()))

	return o.MarshalTo(nil), nil
}

func ErrBadRequest(err error) *errResponse {
	return &errResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
	}
}

func ErrInternal(err error) *errResponse {
	return &errResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
	}
}
