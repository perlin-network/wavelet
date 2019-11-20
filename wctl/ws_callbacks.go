package wctl

import "time"

// OnError called on any WS error
type OnError = func(error)

// Docs: https://wavelet.perlin.net/docs/ws

// Mod: accounts
type (
	BalanceUpdate struct {
		AccountID [32]byte  `json:"account_id"`
		Balance   uint64    `json:"balance"`
		Time      time.Time `json:"time"`
	}
	OnBalanceUpdated = func(BalanceUpdate)

	GasBalanceUpdate struct {
		AccountID  [32]byte  `json:"account_id"`
		GasBalance uint64    `json:"gas_balance"`
		Time       time.Time `json:"time"`
	}
	OnGasBalanceUpdated = func(GasBalanceUpdate)

	NumPagesUpdated struct {
		AccountID [32]byte  `json:"account_id"`
		NumPages  uint64    `json:"num_pages_updated"`
		Time      time.Time `json:"time"`
	}
	OnNumPagesUpdated = func(NumPagesUpdated)

	StakeUpdated struct {
		AccountID [32]byte  `json:"account_id"`
		Stake     uint64    `json:"stake"`
		Time      time.Time `json:"time"`
	}
	OnStakeUpdated = func(StakeUpdated)

	RewardUpdated struct {
		AccountID [32]byte  `json:"account_id"`
		Reward    uint64    `json:"reward"`
		Time      time.Time `json:"time"`
	}
	OnRewardUpdated = func(RewardUpdated)
)

// Mod: network
type (
	PeerUpdate struct {
		AccountID [32]byte  `json:"public_key"`
		Address   string    `json:"address"` // IP:port
		Time      time.Time `json:"time"`
		Message   string    `json:"message"`
	}

	PeerJoin   struct{ PeerUpdate }
	OnPeerJoin = func(PeerJoin)

	PeerLeave   struct{ PeerUpdate }
	OnPeerLeave = func(PeerLeave)
)

// Mod: consensus
type (
	Proposal struct {
		BlockID    [32]byte `json:"block_id"`
		BlockIndex uint64   `json:"block_index"`
		NumTxs     uint64   `json:"num_transactions"`
		Message    string   `json:"message"`
	}
	OnProposal = func(Proposal)

	Finalized struct {
		BlockID     [32]byte `json:"block_id"`
		BlockHeight uint64   `json:"block_index"`
		NumApplied  int      `json:"num_applied_tx"`
		NumRejected int      `json:"num_rejected_tx"`
		NumPruned   int      `json:"num_pruned_tx"`
		Message     string   `json:"message"`
	}
	OnFinalized = func(Finalized)
)

// Mod: stake
type (
	StakeRewardValidator struct {
		Creator      [32]byte  `json:"creator"`
		Recipient    [32]byte  `json:"recipient"`
		CreatorTxID  [32]byte  `json:"creator_tx_id"`
		RewardeeTxID [32]byte  `json:"rewardee_tx_id"`
		Entropy      [32]byte  `json:"entropy"`
		Accuracy     float64   `json:"acc"`
		Threshold    float64   `json:"threshold"`
		Time         time.Time `json:"time"`
		Message      string    `json:"message"`
	}
	OnStakeRewardValidator = func(StakeRewardValidator)
)

// Mod: contract
type (
	ContractGas struct {
		SenderID   [32]byte  `json:"sender_id"`
		ContractID [32]byte  `json:"contract_id"`
		Gas        uint64    `json:"gas"`
		GasLimit   uint64    `json:"gas_limit"`
		Time       time.Time `json:"time"`
		Message    string    `json:"message"`
	}
	OnContractGas = func(ContractGas)

	ContractLog struct {
		ContractID [32]byte  `json:"contract_id"`
		Time       time.Time `json:"time"`
		Message    string    `json:"message"`
	}
	OnContractLog = func(ContractLog)
)

// Mod: tx
type (
	TxApplied struct {
		TxID     [32]byte  `json:"tx_id"`
		SenderID [32]byte  `json:"sender_id"`
		Tag      byte      `json:"tag"`
		Time     time.Time `json:"time"`
	}
	OnTxApplied = func(TxApplied)

	TxGossipError struct {
		Error   string    `json:"error"`
		Time    time.Time `json:"time"`
		Message string    `json:"message"`
	}
	OnTxGossipError = func(TxGossipError)

	TxFailed struct {
		TxID     [32]byte  `json:"tx_id"`
		SenderID [32]byte  `json:"sender_id"`
		Tag      byte      `json:"tag"`
		Error    string    `json:"error"`
		Time     time.Time `json:"time"`
	}
	OnTxFailed = func(TxFailed)
)

// Mod: metrics
type (
	Metrics struct {
		BlocksQueried      uint64    `json:"blocks.queried"`
		BlocksFinalized    float64   `json:"blocks.finalized"` // mean
		TxGossiped         uint64    `json:"tx.gossiped"`
		TxReceived         uint64    `json:"tx.received"`
		TxAccepted         uint64    `json:"tx.accepted"`
		TxDownloaded       uint64    `json:"tx.downloaded"`
		BpsQueried         float64   `json:"bps.queried"`
		TpsGossiped        float64   `json:"tps.gossiped"`
		TpsReceived        float64   `json:"tps.received"`
		TpsAccepted        float64   `json:"tps.accepted"`
		TpsDownloaded      float64   `json:"tps.downloaded"`
		QueryLatencyMaxMS  int64     `json:"query.latency.max.ms"`
		QueryLatencyMinMS  int64     `json:"query.latency.min.ms"`
		QueryLatencyMeanMS float64   `json:"query.latency.mean.ms"`
		Time               time.Time `json:"time"`
		Message            string    `json:"message"`
	}
	OnMetrics = func(Metrics)
)
