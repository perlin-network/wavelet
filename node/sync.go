package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network/rpc"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"time"
)

const SyncQueryPeerNum = 3
const SyncChildrenQueryPeerNum = 5
const SyncBroadcastTxNum = 16

type SyncWorker struct {
	wavelet *Wavelet
}

func NewSyncWorker(wavelet *Wavelet) *SyncWorker {
	return &SyncWorker{
		wavelet: wavelet,
	}
}

// randomlySelectPeers randomly selects N closest peers w.r.t. this node.
func (w *SyncWorker) randomlySelectPeers(n int) ([]string, error) {
	peers := w.wavelet.routes.FindClosestPeers(w.wavelet.net.ID, n+1)

	var addresses []string

	for _, peer := range peers {
		if peer.Address != w.wavelet.net.ID.Address {
			addresses = append(addresses, peer.Address)
		}
	}

	return addresses, nil
}

func (w *SyncWorker) RunSeedBroadcastLoop() {
	for {
		time.Sleep(3 * time.Second)
		var tx *wire.Transaction

		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			selected := l.Graph.Store.GetMostRecentlyUsed(2)
			if len(selected) != 0 {
				raw, err := l.Graph.Store.GetBySymbol(selected[len(selected)-1])
				if err == nil {
					tx = &wire.Transaction{
						Sender:    raw.Sender,
						Nonce:     raw.Nonce,
						Parents:   raw.Parents,
						Tag:       raw.Tag,
						Payload:   raw.Payload,
						Signature: raw.Signature,
					}
				} else {
					log.Error().Err(err).Msg("cannot get tx by symbol")
				}
			}
		})

		if tx == nil {
			continue
		}

		w.wavelet.net.BroadcastRandomly(tx, SyncQueryPeerNum)
	}
}

func (w *SyncWorker) Start() {
	go w.RunSeedBroadcastLoop()
}

func (w *SyncWorker) QueryParents(parents []string) {
	pushHint := make([]string, 0)
	w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
		for _, p := range parents {
			if !l.Store.TransactionExists(p) {
				pushHint = append(pushHint, p)
			}
		}
	})
	w.wavelet.net.BroadcastRandomly(&TxPushHint{
		Transactions: pushHint,
	}, 3)
}

func (w *SyncWorker) QueryChildren(id string) {
	peers, err := w.randomlySelectPeers(SyncChildrenQueryPeerNum)
	if err != nil {
		log.Error().Err(err).Msg("unable to select peers")
		return
	}

	children := make(map[string]struct{})

	for _, p := range peers {
		client, err := w.wavelet.net.Client(p)
		if err != nil {
			log.Error().Err(err).Msg("unable to create client")
			continue
		}

		request := new(rpc.Request)
		request.SetTimeout(10 * time.Second)
		request.SetMessage(&SyncChildrenQueryRequest{
			Id: id,
		})
		_res, err := client.Request(request)
		if err != nil {
			log.Error().Err(err).Msg("request failed")
			continue
		}
		res, ok := _res.(*SyncChildrenQueryResponse)
		if !ok {
			log.Error().Err(err).Msg("response type mismatch")
			continue
		}

		for _, child := range res.Children {
			children[child] = struct{}{}
		}
	}

	deleteList := make([]string, 0)

	w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
		for c, _ := range children {
			if l.Store.TransactionExists(c) {
				deleteList = append(deleteList, c)
			}
		}
	})

	for _, c := range deleteList {
		delete(children, c)
	}

	pushHint := make([]string, 0)
	for c, _ := range children {
		pushHint = append(pushHint, c)
	}

	w.wavelet.net.BroadcastRandomly(&TxPushHint{
		Transactions: pushHint,
	}, 3)
}
