package node

import (
	"bytes"
	"context"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Protocol struct {
	ledger *wavelet.Ledger
	client *skademlia.Client

	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	broadcaster *broadcaster

	gossipStreams map[string]Wavelet_GossipClient
}

func NewProtocol(ledger *wavelet.Ledger, client *skademlia.Client) *Protocol {
	ctx, cancel := context.WithCancel(context.Background())

	p := &Protocol{
		ledger: ledger,
		client: client,

		ctx:    ctx,
		cancel: cancel,

		broadcaster: NewBroadcaster(runtime.NumCPU()*2, 1024),

		gossipStreams: make(map[string]Wavelet_GossipClient),
	}

	p.client.OnPeerJoin(func(conn *grpc.ClientConn, id *skademlia.ID) {
		publicKey := id.PublicKey()

		logger := log.Network("joined")
		logger.Info().
			Str("address", id.Address()).
			Hex("public_key", publicKey[:]).
			Msg("Peer has joined.")
	})

	p.client.OnPeerLeave(func(conn *grpc.ClientConn, id *skademlia.ID) {
		publicKey := id.PublicKey()

		logger := log.Network("left")
		logger.Info().
			Str("address", id.Address()).
			Hex("public_key", publicKey[:]).
			Msg("Peer has disconnected.")
	})

	go p.ledger.Run()

	go p.broadcastGossip()
	go p.broadcastQuery()

	go p.broadcastForwardTX()
	go p.broadcastDownloadTX()
	go p.broadcastDownloadChunk()

	go p.broadcastSync()
	go p.broadcastOutOfSync()

	return p
}

func (p *Protocol) Ledger() *wavelet.Ledger {
	return p.ledger
}

func (p *Protocol) loadGossipStream(conn *grpc.ClientConn) (Wavelet_GossipClient, error) {
	stream, exists := p.gossipStreams[conn.Target()]

	if !exists {
		client := NewWaveletClient(conn)

		stream, err := client.Gossip(context.Background())
		if err != nil {
			return nil, err
		}

		p.gossipStreams[conn.Target()] = stream

		return stream, nil
	}

	return stream, nil
}

func (p *Protocol) broadcastGossip() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.GossipTxOut:
			batch := &TransactionBatch{Transactions: [][]byte{evt.TX.Marshal()}}

			peers := p.client.ClosestPeers()

			for i, peer := range peers {
				stream, err := p.loadGossipStream(peer)
				if err != nil {
					continue
				}

				if err := stream.Send(batch); err != nil {
					delete(p.gossipStreams, peers[i].Target())
					fmt.Println("failed to send gossip:", err)
				}
			}
		}
	}
}

func (p *Protocol) broadcastForwardTX() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.ForwardTxOut:
			batch := &TransactionBatch{Transactions: [][]byte{evt.TX.Marshal()}}

		EMPTY_CHAN:
			for i, n := 0, len(p.ledger.ForwardTxOut); i < n && i < math.MaxUint8; i++ {
				select {
				case evt = <-p.ledger.ForwardTxOut:
				default:
					break EMPTY_CHAN
				}

				batch.Transactions = append(batch.Transactions, evt.TX.Marshal())
			}

			peers := p.client.ClosestPeers()

			for i, peer := range peers {
				stream, err := p.loadGossipStream(peer)
				if err != nil {
					continue
				}

				if err := stream.Send(batch); err != nil {
					delete(p.gossipStreams, peers[i].Target())
					fmt.Println("failed to forward tx:", err)
				}
			}
		}
	}
}

func (p *Protocol) broadcastQuery() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.QueryOut:
			peers, err := SelectPeers(p.client.ClosestPeers(), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while querying")
				continue
			}

			req := &QueryRequest{Round: evt.Round.Marshal()}

			ids := make([]*skademlia.ID, len(peers))
			responses := make([]*QueryResponse, len(peers))

			p.broadcaster.Broadcast(p.ctx, peers, func(i int, client WaveletClient) error {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				p := &peer.Peer{}

				res, err := client.Query(ctx, req, grpc.Peer(p))
				if err != nil {
					return err
				}

				info := noise.InfoFromPeer(p)

				if info == nil {
					return errors.New("auth info is nil")
				}

				ids[i] = info.Get(skademlia.KeyID).(*skademlia.ID)

				responses[i] = res

				return nil
			})

			votes := make([]wavelet.VoteQuery, len(responses))

			for i := range responses {
				if responses[i] == nil || ids[i] == nil || len(responses[i].Round) == 0 {
					continue
				}

				round, err := wavelet.UnmarshalRound(bytes.NewReader(responses[i].Round))
				if err != nil {
					fmt.Println("error while unmarshaling query response:", err)
					continue
				}

				votes[i].Preferred = round
				votes[i].Voter = ids[i].PublicKey()
			}

			evt.Result <- votes
		}
	}
}

func (p *Protocol) broadcastSync() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.SyncInitOut:
			peers, err := SelectPeers(p.client.ClosestPeers(), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers to start syncing with")
				continue
			}

			req := &SyncRequest{}

			ids := make([]*skademlia.ID, len(peers))
			responses := make([]*SyncResponse, len(peers))

			p.broadcaster.Broadcast(p.ctx, peers, func(i int, client WaveletClient) error {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				p := &peer.Peer{}

				res, err := client.InitializeSync(ctx, req, grpc.Peer(p))
				if err != nil {
					return err
				}

				info := noise.InfoFromPeer(p)

				if info == nil {
					return errors.New("auth info is nil")
				}

				ids[i] = info.Get(skademlia.KeyID).(*skademlia.ID)

				responses[i] = res

				return nil
			})

			votes := make([]wavelet.SyncInitMetadata, len(responses))

			for i := range responses {
				if responses[i] == nil || ids[i] == nil {
					continue
				}

				hashes := make([][blake2b.Size256]byte, len(responses[i].ChunkHashes))

				for j := range responses[i].ChunkHashes {
					copy(hashes[j][:], responses[i].ChunkHashes[j])
				}

				votes[i].ChunkHashes = hashes
				votes[i].RoundID = responses[i].LatestRoundId
				votes[i].PeerID = ids[i]
			}

			evt.Result <- votes
		}
	}
}

func (p *Protocol) broadcastOutOfSync() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.OutOfSyncOut:
			peers, err := SelectPeers(p.client.ClosestPeers(), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers for out of sync check")
				continue
			}

			req := &OutOfSyncRequest{}

			ids := make([]*skademlia.ID, len(peers))
			responses := make([]*OutOfSyncResponse, len(peers))

			p.broadcaster.Broadcast(p.ctx, peers, func(i int, client WaveletClient) error {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				p := &peer.Peer{}

				res, err := client.CheckOutOfSync(ctx, req, grpc.Peer(p))
				if err != nil {
					return err
				}

				info := noise.InfoFromPeer(p)

				if info == nil {
					return errors.New("auth info is nil")
				}

				ids[i] = info.Get(skademlia.KeyID).(*skademlia.ID)

				responses[i] = res

				return nil
			})

			votes := make([]wavelet.VoteOutOfSync, len(responses))

			for i := range responses {
				if responses[i] == nil || ids[i] == nil {
					continue
				}

				round, err := wavelet.UnmarshalRound(bytes.NewReader(responses[i].Round))
				if err != nil {
					fmt.Println("error unmarshaling out of sync response:", err)
					continue
				}

				votes[i].Round = round
				votes[i].Voter = ids[i].PublicKey()
			}

			evt.Result <- votes
		}
	}
}

func (p *Protocol) broadcastDownloadTX() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.DownloadTxOut:
			peers, err := SelectPeers(p.client.ClosestPeers(), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers for out of sync check")
				continue
			}

			req := &DownloadTxRequest{Ids: make([][]byte, len(evt.IDs))}

			for i := range evt.IDs {
				req.Ids[i] = evt.IDs[i][:]
			}

			responses := make([]*DownloadTxResponse, len(peers))

			p.broadcaster.Broadcast(p.ctx, peers, func(i int, client WaveletClient) error {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()

				res, err := client.DownloadTx(ctx, req)
				if err != nil {
					return err
				}

				responses[i] = res

				return nil
			})

			set := make(map[common.TransactionID]wavelet.Transaction)

			for i := range responses {
				if responses[i] == nil {
					continue
				}

				for _, buf := range responses[i].Transactions {
					tx, err := wavelet.UnmarshalTransaction(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("error while unmarshaling download tx response:", err)
						continue
					}

					set[tx.ID] = tx
				}
			}

			transactions := make([]wavelet.Transaction, 0, len(set))

			for _, tx := range set {
				transactions = append(transactions, tx)
			}

			evt.Result <- transactions
		}
	}
}

func (p *Protocol) broadcastDownloadChunk() {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.SyncDiffOut:
			collected := make([][]byte, len(evt.Sources))

			var count uint32
			var wg sync.WaitGroup
			wg.Add(len(evt.Sources))

			for chunkID, src := range evt.Sources {
				chunkID, src := chunkID, src

				req := &DownloadChunkRequest{Id: src.Hash[:]}

				go func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						conn, err := p.client.Dial(src.PeerIDs[rand.Intn(len(src.PeerIDs))].Address())

						if err != nil {
							continue
						}

						client := NewWaveletClient(conn)

						diff, err := func() ([]byte, error) {
							ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
							defer cancel()

							res, err := client.DownloadChunk(ctx, req)
							if err != nil {
								return nil, err
							}

							if len(res.Chunk) == 0 || blake2b.Sum256(res.Chunk) != src.Hash {
								return nil, errors.Errorf("chunk of size %d is invalid (expected %x, but got %x)", len(res.Chunk), src.Hash, blake2b.Sum256(res.Chunk))
							}

							return res.Chunk, nil
						}()

						if err != nil {
							fmt.Println("failed to get chunk:", err)
							continue
						}

						collected[chunkID] = diff
						atomic.AddUint32(&count, 1)

						break
					}
				}()
			}

			wg.Wait()

			if atomic.LoadUint32(&count) != uint32(len(evt.Sources)) {
				evt.Error <- errors.New("failed to fetch some chunks from our peers")
				continue
			}

			evt.Result <- collected
		}
	}
}

func (p *Protocol) Gossip(stream Wavelet_GossipServer) error {
	for {
		batch, err := stream.Recv()

		if err != nil {
			return err
		}

		var transactions []wavelet.Transaction

		for _, buf := range batch.Transactions {
			tx, err := wavelet.UnmarshalTransaction(bytes.NewReader(buf))

			if err != nil {
				continue
			}

			transactions = append(transactions, tx)
		}

		evt := wavelet.EventIncomingGossip{TXs: transactions}

		select {
		case <-stream.Context().Done():
			return nil
		case <-time.After(1 * time.Second):
			fmt.Println("timed out sending gossip request to ledger")
		case p.ledger.GossipTxIn <- evt:
		}
	}
}

func (p *Protocol) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	round, err := wavelet.UnmarshalRound(bytes.NewReader(req.Round))
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal round")
	}

	res := &QueryResponse{}

	evt := wavelet.EventIncomingQuery{Round: round, Response: make(chan *wavelet.Round, 1), Error: make(chan error, 1)}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending query request to ledger")
		return res, nil
	case p.ledger.QueryIn <- evt:
	}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting query result from ledger")
	case err := <-evt.Error:
		if err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
			p, ok := peer.FromContext(ctx)

			if !ok {
				return res, nil
			}

			fmt.Printf("got an error processing query request from %s: %s\n", p.Addr, err)
		}
	case preferred := <-evt.Response:
		if preferred != nil {
			res.Round = preferred.Marshal()
		}
	}

	return res, nil
}

func (p *Protocol) InitializeSync(ctx context.Context, req *SyncRequest) (*SyncResponse, error) {
	evt := wavelet.EventIncomingSyncInit{RoundID: req.RoundId, Response: make(chan wavelet.SyncInitMetadata, 1)}

	res := &SyncResponse{}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync init request to ledger")
		return res, nil
	case p.ledger.SyncInitIn <- evt:
	}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync init results from ledger")
	case data := <-evt.Response:
		res.ChunkHashes = make([][]byte, len(data.ChunkHashes))

		for i, hash := range data.ChunkHashes {
			res.ChunkHashes[i] = hash[:]
		}

		res.LatestRoundId = data.RoundID
	}

	return res, nil
}

func (p *Protocol) CheckOutOfSync(ctx context.Context, _ *OutOfSyncRequest) (*OutOfSyncResponse, error) {
	evt := wavelet.EventIncomingOutOfSyncCheck{Response: make(chan *wavelet.Round, 1)}

	res := &OutOfSyncResponse{}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending out of sync check to ledger")
		return res, nil
	case p.ledger.OutOfSyncIn <- evt:
	}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting out of sync check results from ledger")
	case round := <-evt.Response:
		res.Round = round.Marshal()
	}

	return res, nil
}

func (p *Protocol) DownloadTx(ctx context.Context, req *DownloadTxRequest) (*DownloadTxResponse, error) {
	ids := make([]common.TransactionID, len(req.Ids))

	for i, id := range req.Ids {
		copy(ids[i][:], id)
	}

	res := &DownloadTxResponse{}

	evt := wavelet.EventIncomingDownloadTX{IDs: ids, Response: make(chan []wavelet.Transaction, 1)}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending download tx request to ledger")
		return res, nil
	case p.ledger.DownloadTxIn <- evt:
	}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting download tx results from ledger")
	case txs := <-evt.Response:
		res.Transactions = make([][]byte, len(txs))

		for i, tx := range txs {
			res.Transactions[i] = tx.Marshal()
		}
	}

	return res, nil
}

func (p *Protocol) DownloadChunk(ctx context.Context, req *DownloadChunkRequest) (*DownloadChunkResponse, error) {
	evt := wavelet.EventIncomingSyncDiff{Response: make(chan []byte, 1)}
	copy(evt.ChunkHash[:], req.Id)

	res := &DownloadChunkResponse{}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync diff request to ledger")
		return res, nil
	case p.ledger.SyncDiffIn <- evt:
	}

	select {
	case <-ctx.Done():
		return res, nil
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync diff results from ledger")
	case res.Chunk = <-evt.Response:
	}

	return res, nil
}
