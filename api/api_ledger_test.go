package api_test

import (
	"encoding/hex"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/crypto"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	apiClient "github.com/perlin-network/wavelet/cmd/wctl/client"
	"github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/payload"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)

const (
	host           = "localhost"
	privateKeyFile = "../cmd/wavelet/wallet.txt"
	txID           = "bd0ed50ef81a40a233fad3a3cc7dee53880764a10facc81078e0f46b9f7d141a"
	recipient      = "84c546dddfa01833bf9bd3478bdd3d8a1280725f081cbb70410a77d0ae471d88"
)

func setupMockServer(port int, privateKeyFile string, mockWavelet node.NodeInterface) (*http.Server, *apiClient.Client, error) {
	privateKeyBytes, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, nil, err
	}
	keys, err := crypto.FromPrivateKey(security.SignaturePolicy, string(privateKeyBytes))
	if err != nil {
		return nil, nil, err
	}
	sc := make(chan *http.Server)
	go api.Run(nil, mockWavelet, sc, api.Options{
		ListenAddr: fmt.Sprintf("%s:%d", "localhost", port),
		Clients: []*api.ClientInfo{
			&api.ClientInfo{
				PublicKey: keys.PublicKeyHex(),
				Permissions: api.ClientPermissions{
					CanSendTransaction: true,
					CanPollTransaction: true,
					CanControlStats:    true,
					CanAccessLedger:    true,
				},
			},
		},
	})
	server := <-sc
	time.Sleep(50 * time.Millisecond)

	client, err := client(port, privateKeyFile)
	if err != nil {
		return nil, nil, err
	}
	return server, client, nil
}

func client(port int, privateKeyFile string) (*apiClient.Client, error) {
	privateKeyBytes, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to open api private key file: %s", privateKeyFile)
	}

	client, err := apiClient.NewClient(apiClient.Config{
		APIHost:    host,
		APIPort:    uint(port),
		PrivateKey: string(privateKeyBytes),
		UseHTTPS:   false,
	})
	if err != nil {
		return nil, err
	}

	err = client.Init()
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetRandomUnusedPort() int {
	listener, _ := net.Listen("tcp", ":0")
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

////////////////////////////////

func Test_api_list_transaction(t *testing.T) {
	numTx := 10
	numLimit := uint64(5)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().PaginateTransactions(uint64(numTx), uint64(numLimit)).Return(func() []*database.Transaction {
		result := []*database.Transaction{}
		for i := 0; i < numTx; i++ {
			result = append(result, &database.Transaction{
				Nonce: uint64(i + numTx),
			})
		}
		return result
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)

	assert.Nil(t, err)
	defer s.Close()

	res, err := c.ListTransaction(uint64(numTx), numLimit)
	assert.Nil(t, err)
	assert.Equal(t, numTx, len(res))
	for i := 0; i < numTx; i++ {
		assert.Equal(t, uint64(i+numTx), res[i].Nonce)
	}
}

func Test_api_execute_contract(t *testing.T) {
	paramEntry := "entry"
	paramParam := []byte("param")
	txIDBytes, _ := hex.DecodeString(txID)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().ExecuteContract(txIDBytes, paramEntry, paramParam).Return(func() ([]byte, error) {
		return []byte("result"), nil
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.ExecuteContract(txID, paramEntry, paramParam)
	assert.Nil(t, err)
	assert.Equal(t, []byte("result"), res.Result)
}

func Test_api_get_contract(t *testing.T) {
	txIDBytes, _ := hex.DecodeString(txID)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().LoadContract(txIDBytes).Return(func() ([]byte, error) {
		return []byte("contract-" + txID), nil
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.GetContract(txID, "/tmp/filename1")
	assert.Nil(t, err)
	assert.Equal(t, txID, res.TransactionID)
	assert.Equal(t, []byte("contract-"+txID), res.Code)
}

func Test_api_send_contract(t *testing.T) {

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)

	mockNode.EXPECT().MakeTransaction(gomock.Any(), gomock.Any()).Return(func() *wire.Transaction {
		return &wire.Transaction{}
	}()).Times(1)
	mockNode.EXPECT().BroadcastTransaction(gomock.Any()).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	filename := "/tmp/filename2"
	assert.Nil(t, ioutil.WriteFile(filename, []byte("hello\ngo\n"), 0644))

	res, err := c.SendContract(filename)
	assert.Nil(t, err)
	assert.True(t, len(res.TransactionID) > 0)
}

func Test_api_list_contracts(t *testing.T) {
	numContracts := 10

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().NumContracts().Return(func() uint64 {
		return 1000
	}()).Times(1)
	mockLedger.EXPECT().PaginateContracts(uint64(1000-50), uint64(50)).Return(func() []*wavelet.Contract {
		result := []*wavelet.Contract{}
		for i := 0; i < numContracts; i++ {
			result = append(result, &wavelet.Contract{
				TransactionID: fmt.Sprintf("%d", i+numContracts),
				Code:          []byte("code"),
			})
		}
		return result
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.ListContracts(nil, nil)
	assert.Nil(t, err)
	assert.Equal(t, numContracts, len(res))
	for i, contract := range res {
		assert.Equal(t, fmt.Sprintf("%d", i+numContracts), contract.TransactionID)
	}
}

func Test_api_ledger_state(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().Snapshot().Return(func() map[string]interface{} {
		return map[string]interface{}{
			"key": "value",
		}
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.LedgerState()
	assert.Nil(t, err)
	assert.Equal(t, "value", res.State["key"])
}

func Test_api_send_transaction(t *testing.T) {
	recipientDecoded, _ := hex.DecodeString(recipient)
	tag := uint32(params.TagGeneric)
	writer := payload.NewWriter(nil)
	writer.WriteBytes(recipientDecoded)
	writer.WriteUint64(uint64(10))
	writer.WriteString("balance")
	writer.WriteUint32(25)
	pl := writer.Bytes()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)

	mockNode.EXPECT().MakeTransaction(tag, pl).Times(1).Return(func() *wire.Transaction {
		reader := payload.NewReader(pl)
		r, err := reader.ReadBytes()
		assert.Nil(t, err)
		assert.Equal(t, recipientDecoded, r)
		a, err := reader.ReadUint64()
		assert.Nil(t, err)
		assert.Equal(t, uint64(10), a)
		s, err := reader.ReadString()
		assert.Nil(t, err)
		assert.Equal(t, "balance", s)
		f, err := reader.ReadUint32()
		assert.Nil(t, err)
		assert.Equal(t, uint32(25), f)
		return &wire.Transaction{
			Tag:     tag,
			Payload: pl,
		}
	}()).Times(1)
	mockNode.EXPECT().BroadcastTransaction(gomock.Any()).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.SendTransaction(tag, pl)
	assert.Nil(t, err)
	assert.True(t, len(res.TransactionID) > 0)
}

func Test_api_get_transaction(t *testing.T) {
	txIDBytes, _ := hex.DecodeString(txID)
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().GetBySymbol(txIDBytes).Return(func() (*database.Transaction, error) {
		return &database.Transaction{
			Nonce: uint64(123),
		}, nil
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.GetTransaction(txID)
	assert.Nil(t, err)
	assert.Equal(t, uint64(123), res.Nonce)
}

func Test_api_get_account(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().Accounts().Return(func() *wavelet.Accounts {
		return nil
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.LoadAccount(txID)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(res))
}

func Test_api_find_parents(t *testing.T) {
	numParents := 5

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().FindEligibleParents().Return(func() (parents [][]byte, err error) {
		for i := 0; i < numParents; i++ {
			parents = append(parents, []byte(fmt.Sprintf("id-%d", i)))
		}
		return parents, nil
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.FindParents()
	assert.Nil(t, err)
	assert.Equal(t, numParents, len(res.ParentIDs))
	for i, val := range res.ParentIDs {
		assert.Equal(t, hex.EncodeToString([]byte(fmt.Sprintf("id-%d", i))), val)
	}
}

func Test_api_forward_transaction(t *testing.T) {
	wiredTx := &wire.Transaction{
		Sender: func() []byte {
			decoded, _ := hex.DecodeString("71e6c9b83a7ef02bae6764991eefe53360a0a09be53887b2d3900d02c00a3858")
			return []byte(decoded)
		}(),
		Nonce: uint64(2),
		Parents: func() [][]byte {
			decoded, _ := hex.DecodeString(txID)
			return [][]byte{decoded}
		}(),
		Tag:     uint32(params.TagGeneric),
		Payload: []byte("payload"),
		Signature: func() []byte {
			decoded, _ := hex.DecodeString("38609fc546cb54db22f6ed14608609cf6a07642c71b5b75bfc9859cc0fd7cb730140a31926e4b0a33be96b008e207c4266da0bfbae3db116009f7f6ea629e205")
			return []byte(decoded)
		}(),
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockNode := node.NewMockNodeInterface(mockCtrl)
	mockLedger := wavelet.NewMockLedgerInterface(mockCtrl)

	mockLedger.EXPECT().FindEligibleParents().Return(func() (parents [][]byte, err error) {
		decoded, _ := hex.DecodeString(txID)
		return [][]byte{decoded}, nil
	}()).Times(1)
	mockNode.EXPECT().LedgerDo(gomock.Any()).Times(1).DoAndReturn(func(f func(ledger wavelet.LedgerInterface)) {
		f(mockLedger)
	}).Times(1)
	mockNode.EXPECT().BroadcastTransaction(wiredTx).Times(1)

	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, mockNode)
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.ForwardTransaction(wiredTx.Nonce, wiredTx.Tag, wiredTx.Payload)
	assert.Nil(t, err)
	assert.True(t, len(res.TransactionID) > 0)
}

func Test_api_serverVersion(t *testing.T) {
	port := GetRandomUnusedPort()
	s, c, err := setupMockServer(port, privateKeyFile, node.NewMockNodeInterface(gomock.NewController(t)))
	assert.Nil(t, err)
	defer s.Close()

	res, err := c.ServerVersion()
	assert.Nil(t, err)
	assert.Equal(t, params.Version, res.Version)
}
