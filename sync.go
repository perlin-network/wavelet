package wavelet

import (
	"bytes"
	"context"
	"fmt"
	"github.com/djherbis/buffer"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/internal/backoff"
	"github.com/perlin-network/wavelet/internal/filebuffer"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.uber.org/atomic"
	"golang.org/x/crypto/blake2b"
	"io"
	"math/rand"
	"sync"
	"time"
)

type SyncManager struct {
	client   *skademlia.Client
	accounts *Accounts
	blocks   *Blocks

	filePool *filebuffer.Pool

	logger zerolog.Logger
	exit   chan struct{}

	OnOutOfSync []func()
	OnSynced    []func(block Block)
}

func NewSyncManager(
	client *skademlia.Client, accounts *Accounts, blocks *Blocks, filePool *filebuffer.Pool,
) *SyncManager {
	return &SyncManager{
		client:   client,
		accounts: accounts,
		blocks:   blocks,

		filePool: filePool,

		logger: log.Sync("sync"),
		exit:   make(chan struct{}),
	}
}

func (s *SyncManager) Stop() {
	close(s.exit)
}

func (s *SyncManager) Start() {
	b := &backoff.Backoff{Min: 0 * time.Second, Max: 3 * time.Second, Factor: 1.25, Jitter: true}

	for {
		for {
			if !s.stateOutdated() {
				s.wait(b.Duration())
				continue
			}

			break
		}

		for _, fn := range s.OnOutOfSync {
			fn()
		}

		var (
			block Block
			err   error
		)

		for {
			if block, err = s.sync(b); err != nil {
				fmt.Println(err)
				continue
			}

			break
		}

		for _, fn := range s.OnSynced {
			fn(block)
		}
	}
}

func (s *SyncManager) sync(b *backoff.Backoff) (Block, error) {
	b.Reset()

	var (
		peers []syncPeer

		block     Block
		checksums [][blake2b.Size256]byte
		streams   []Wavelet_SyncClient

		err error
	)

	for {
		peers, err = s.findPeersToDownloadStateFrom(conf.GetSnowballK())
		if err != nil {
			s.wait(b.Duration())
			continue
		}

		block, checksums, streams, err = s.collateLatestStateDetails(peers)
		if err != nil {
			s.wait(b.Duration())
			continue
		}

		break
	}

	b.Reset()

	chunksBuffer, err := s.filePool.GetBounded(int64(len(checksums)) * sys.SyncChunkSize)
	if err != nil {
		return block, err
	}

	diffBuffer := s.filePool.GetUnbounded()

	defer s.filePool.Put(chunksBuffer)
	defer s.filePool.Put(diffBuffer)

	err = nil

	for i := 0; i < 3; i++ {
		if err = s.downloadStateInChunks(checksums, streams, chunksBuffer, diffBuffer); err != nil {
			s.wait(b.Duration())
			continue
		}

		break
	}

	if err != nil {
		return block, err
	}

	b.Reset()

	snapshot := s.accounts.Snapshot()

	if err := snapshot.ApplyDiff(diffBuffer); err != nil {
		return block, err
	}

	if checksum := snapshot.Checksum(); checksum != block.Merkle {
		return block, errors.Errorf("got merkle root %x but expected %x", checksum, block.Merkle)
	}

	if _, err := s.blocks.Save(&block); err != nil {
		return block, err
	}

	if err := s.accounts.Commit(snapshot); err != nil {
		return block, err
	}

	s.logger.Info().
		Int("num_chunks", len(checksums)).
		Uint64("new_block_height", block.Index).
		Hex("new_block_id", block.ID[:]).
		Hex("new_merkle_root", block.Merkle[:]).
		Msg("Successfully built a new state snapshot out of chunk(s) we have received from peers.")

	return block, nil
}

/** Methods that help us figure out whether or not our node is outdated. */

func (s *SyncManager) stateOutdated() bool {
	samplerK := conf.GetSnowballK()

	// Our initial belief is that we're not outdated.
	sampler := NewSnowball()
	sampler.Prefer(&syncVote{outOfSync: false})

	// Run a worker to consolidate votes from our peers as to whether or not our state is outdated.
	votes := make(chan Vote, samplerK)
	go s.consolidateVotesFromPeers(sampler, votes)

	for { // Infinitely keep asking our peers if we are outdated given our latest state.
		converged := s.collectVotesFromPeers(sampler, votes, samplerK)

		if converged {
			break
		}

		s.wait(5 * time.Millisecond)
	}

	return sampler.preferred.(*syncVote).outOfSync
}

// Ask a peer if our latest block height is far too outdated.
func (s *SyncManager) askIfWereOutdated(ctx context.Context, peer skademlia.ClosestPeer, height uint64) (bool, error) {
	res, err := NewWaveletClient(peer.Conn()).CheckOutOfSync(ctx, &OutOfSyncRequest{BlockIndex: height})
	if err != nil {
		return false, errors.Wrap(err, "failed to ask peer if they believe we are out-of-sync")
	}

	return res.OutOfSync, nil
}

// Collect votes from peers, and return true if our sampling algorithm has terminated. Else, return false.
func (s *SyncManager) collectVotesFromPeers(sampler *Snowball, votes chan<- Vote, samplerK int) bool {
	latestHeight := s.blocks.LatestHeight()

	peers, err := SelectPeers(s.client.ClosestPeers(), samplerK)
	if err != nil {
		s.logger.Warn().Msg("It looks like there are no peers for us to sync with. Retrying after 1 second...")
		s.wait(1 * time.Second)

		return false
	}

	var wg sync.WaitGroup

	wg.Add(len(peers))

	for _, p := range peers {
		go func(peer skademlia.ClosestPeer) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			maybe, err := s.askIfWereOutdated(ctx, peer, latestHeight)
			if err != nil {
				return
			}

			votes <- &syncVote{voter: peer.ID(), outOfSync: maybe}
		}(p)
	}

	wg.Wait()

	// Our sampling algorithm has terminated: stop processing votes and see if our sampler
	// has concluded that the majority of the network has told us to sync to the latest state.

	if sampler.Decided() {
		close(votes)
		return true
	}

	return false
}

// Consolidate votes from our peers to figure out if our latest state if out-of-date.
func (s *SyncManager) consolidateVotesFromPeers(sampler *Snowball, votes <-chan Vote) {
	slice := make([]Vote, 0, cap(votes))
	voters := make(map[AccountID]struct{}, cap(votes))

	for vote := range votes {
		vote := vote.(*syncVote)

		if _, recorded := voters[vote.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[vote.voter.PublicKey()] = struct{}{}

		slice = append(slice, vote)

		if len(slice) == cap(slice) {
			sampler.Tick(calculateTallies(s.accounts, slice))

			voters = make(map[AccountID]struct{}, cap(votes))
			slice = slice[:0]
		}
	}
}

/** Methods for updating our node to the latest state available from the network. */

type syncPeer struct {
	peer   skademlia.ClosestPeer
	stream Wavelet_SyncClient

	block     Block
	checksums [][blake2b.Size256]byte
}

// Find and establish sessions with a fixed number of peers to download the latest state from.
func (s *SyncManager) findPeersToDownloadStateFrom(numPeers int) ([]syncPeer, error) {
	sessions := make([]syncPeer, 0, numPeers)
	sessionsLock := sync.Mutex{}

	peers, err := SelectPeers(s.client.ClosestPeers(), numPeers)
	if err != nil {
		s.logger.Warn().Msg("It looks like there are no peers for us to sync with. Retrying after 1 second...")
		s.wait(1 * time.Second)

		return nil, errors.New("no peers for us to sync with")
	}

	height := s.blocks.LatestHeight()
	req := &SyncRequest{Data: &SyncRequest_BlockId{BlockId: height}}

	var wg sync.WaitGroup

	wg.Add(len(peers))

	for _, p := range peers {
		go func(peer skademlia.ClosestPeer) {
			defer wg.Done()

			stream, err := NewWaveletClient(peer.Conn()).Sync(context.Background())
			if err != nil {
				return
			}

			block, checksums, err := s.askForLatestStateDetails(stream, height, req)
			if err != nil {
				return
			}

			sessionsLock.Lock()
			defer sessionsLock.Unlock()

			sessions = append(sessions, syncPeer{peer: peer, stream: stream, block: block, checksums: checksums})
		}(p)
	}

	wg.Wait()

	if len(sessions) < numPeers {
		return nil, errors.Errorf(
			"got %d sessions established but require a minimum of %d sessions to sync",
			len(sessions),
			numPeers,
		)
	}

	return sessions, nil
}

// Ask for the latest block and state Merkle root from a peer.
func (s *SyncManager) askForLatestStateDetails(
	stream Wavelet_SyncClient, height uint64, req *SyncRequest,
) (block Block, checksums [][blake2b.Size256]byte, err error) {
	if err := stream.Send(req); err != nil {
		return Block{}, nil, err
	}

	res, err := stream.Recv()
	if err != nil {
		return Block{}, nil, err
	}

	info := res.GetHeader()
	if info == nil {
		return Block{}, nil, err
	}

	if len(info.Block) == 0 || len(info.Checksums) == 0 {
		return Block{}, nil, errors.New("corrupt sync header")
	}

	block, err = UnmarshalBlock(bytes.NewReader(info.Block))
	if err != nil {
		return Block{}, nil, err
	}

	if block.Index <= height {
		return Block{}, nil,
			errors.Errorf(
				"peers reported latest state is at height %d, but our "+
					"current height is %d and thus our peer is outdated",
				block.Index,
				height,
			)
	}

	checksums = make([][blake2b.Size256]byte, len(info.Checksums))

	for i, buf := range info.Checksums {
		if len(buf) != blake2b.Size256 {
			return Block{}, nil, errors.Errorf("checksum %d was len %d, but expected len %d", i, len(buf), blake2b.Size256)
		}

		copy(checksums[i][:], buf)
	}

	return block, checksums, nil
}

func (s *SyncManager) collateLatestStateDetails(peers []syncPeer) (
	block Block, checksums [][blake2b.Size256]byte, streams []Wavelet_SyncClient, err error,
) {
	var max []byte

	counts := make(map[string]int)
	clients := make(map[string][]Wavelet_SyncClient)

	for _, peer := range peers {
		key := peer.block.ID[:]
		for _, checksum := range peer.checksums {
			key = append(key, checksum[:]...)
		}

		clients[string(key)] = append(clients[string(key)], peer.stream)
		counts[string(key)]++

		if max == nil || counts[string(key)] > counts[string(max)] {
			max = key
			block = peer.block
			checksums = peer.checksums
		}
	}

	if max == nil || checksums == nil { // How...?
		return block, checksums, streams, errors.New("something unexpected happened")
	}

	key := block.ID[:]
	for _, checksum := range checksums {
		key = append(key, checksum[:]...)
	}

	if counts[string(key)] < 2*len(peers)/3 {
		return block, checksums, streams, errors.Errorf(
			"majority of peers are not on the same state: got %d peers on the current state, but need a minimum of %d peers",
			counts[string(key)],
			2*len(peers)/3,
		)
	}

	return block, checksums, clients[string(key)], nil
}

func (s *SyncManager) downloadStateInChunks(
	checksums [][blake2b.Size256]byte,
	streams []Wavelet_SyncClient,
	chunksBuffer buffer.BufferAt,
	diffBuffer io.Writer,
) error {
	mutices := make(map[Wavelet_SyncClient]*sync.Mutex)

	var (
		muticesLock sync.Mutex
		chunkLock   sync.Mutex

		downloadedCount atomic.Uint32
		downloadedSize  atomic.Uint32
	)

	s.logger.Debug().
		Int("num_chunks", len(checksums)).
		Msg("Starting up workers to downloaded all chunks of data needed to sync to the latest block...")

	var wg sync.WaitGroup

	wg.Add(len(checksums))

	for i, checksum := range checksums {
		go func(i int, checksum [blake2b.Size256]byte) {
			defer wg.Done()

			for range streams {
				stream := streams[rand.Intn(len(streams))]

				muticesLock.Lock()
				if _, exists := mutices[stream]; !exists {
					mutices[stream] = &sync.Mutex{}
				}
				mutex := mutices[stream]
				muticesLock.Unlock()

				mutex.Lock()
				chunk, err := s.downloadStateChunk(checksum, stream)
				if err != nil {
					mutex.Unlock()
					continue
				}
				mutex.Unlock()

				chunkLock.Lock()
				_, err = chunksBuffer.WriteAt(chunk, int64(i)*sys.SyncChunkSize)
				chunkLock.Unlock()

				if err != nil {
					continue
				}

				downloadedCount.Add(1)
				downloadedSize.Add(uint32(len(chunk)))

				break
			}
		}(i, checksum)
	}

	wg.Wait()

	if d := downloadedCount.Load(); int(d) < len(checksums) {
		return errors.Errorf("only downloaded %d out of %d chunk(s) successfully", d, len(checksums))
	}

	if _, err := io.CopyN(diffBuffer, chunksBuffer, int64(downloadedSize.Load())); err != nil {
		return err
	}

	return nil
}

func (s *SyncManager) downloadStateChunk(checksum [blake2b.Size256]byte, stream Wavelet_SyncClient) ([]byte, error) {
	req := &SyncRequest{Data: &SyncRequest_Checksum{Checksum: checksum[:]}}

	if err := stream.Send(req); err != nil {
		return nil, err
	}

	res, err := stream.Recv()
	if err != nil {
		return nil, err
	}

	chunk := res.GetChunk()
	if chunk == nil {
		return nil, errors.New("peer did not send chunk")
	}

	if len(chunk) > conf.GetSyncChunkSize() {
		return nil, errors.Errorf(
			"got chunk of size %d but chunk can be no larger than %d bytes",
			len(chunk),
			conf.GetSyncChunkSize(),
		)
	}

	recovered := blake2b.Sum256(chunk)

	if recovered != checksum {
		return nil, errors.Errorf(
			"chunk downloaded was hashed to %x, but was trying to download chunk with a has of %x",
			recovered,
			checksum,
		)
	}

	return chunk, nil
}

/** Helper utilities. */

func (s *SyncManager) wait(t time.Duration) {
	timer := time.NewTimer(t)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-s.exit:
	}
}
