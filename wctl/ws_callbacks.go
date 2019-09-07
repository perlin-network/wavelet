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
		Address   string    `json:"string"` // IP:port
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
	RoundEnd struct {
		NumAppliedTx  uint64    `json:"num_applied_tx"`
		NumRejectedTx uint64    `json:"num_rejected_tx"`
		NumIgnoredTx  uint64    `json:"num_ignored_tx"`
		OldRound      uint64    `json:"old_round"`
		NewRound      uint64    `json:"new_round"`
		OldDifficulty uint64    `json:"old_difficulty"`
		NewDifficulty uint64    `json:"new_difficulty"`
		NewRoot       [32]byte  `json:"new_root"`
		OldRoot       [32]byte  `json:"old_root"`
		NewMerkleRoot [16]byte  `json:"new_merkle_root"`
		OldMerkleRoot [16]byte  `json:"old_merkle_root"`
		RoundDepth    int64     `json:"round_depth"`
		Time          time.Time `json:"time"`
		Message       string    `json:"message"`
	}
	OnRoundEnd = func(RoundEnd)

	Prune struct {
		NumTx          uint64    `json:"num_tx"`
		CurrentRoundID [32]byte  `json:"current_round_id"`
		PrunedRoundID  [32]byte  `json:"pruned_round_id"`
		Time           time.Time `json:"time"`
		Message        string    `json:"message"`
	}
	OnPrune = func(Prune)
)

// Mod: stake
type (
	Stake struct {
		Creator      [32]byte  `json:"creator"`
		Recipient    [32]byte  `json:"recipient"`
		CreatorTxID  [32]byte  `json:"creator_tx_id"`
		RewardeeTxID [32]byte  `json:"rewardee_tx_id"`
		Entropy      [32]byte  `json:"entropy"`
		Acc          float64   `json:"acc"`
		Threshold    float64   `json:"threshold"`
		Time         time.Time `json:"time"`
		Message      string    `json:"message"`
	}
	OnStake = func(Stake)
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
		TxID      [32]byte  `json:"tx_id"`
		SenderID  [32]byte  `json:"sender_id"`
		CreatorID [32]byte  `json:"creator_id"`
		Depth     uint64    `json:"depth"`
		Tag       byte      `json:"tag"`
		Time      time.Time `json:"time"`
	}
	OnTxApplied = func(TxApplied)

	TxGossipError struct {
		Error   string    `json:"error"`
		Time    time.Time `json:"time"`
		Message string    `json:"message"`
	}
	OnTxGossipError = func(TxGossipError)

	TxFailed struct {
		TxID      [32]byte  `json:"tx_id"`
		SenderID  [32]byte  `json:"sender_id"`
		CreatorID [32]byte  `json:"creator_id"`
		Depth     uint64    `json:"depth"`
		Tag       byte      `json:"tag"`
		Error     string    `json:"error"`
		Time      time.Time `json:"time"`
	}
	OnTxFailed = func(TxFailed)
)

// Mod: metrics
type (
	Metrics struct {
		RoundQueried       uint64  `json:"round.queried"`
		TxGossiped         uint64  `json:"tx.gossiped"`
		TxReceived         uint64  `json:"tx.received"`
		TxAccepted         uint64  `json:"tx.accepted"`
		TxDownloaded       uint64  `json:"tx.downloaded"`
		RpsQueried         float64 `json:"rps.queried"`
		TpsGossiped        float64 `json:"tps.gossiped"`
		TpsReceived        float64 `json:"tps.received"`
		TpsAccepted        float64 `json:"tps.accepted"`
		TpsDownloaded      uint64  `json:"tps.downloaded"`
		QueryLatencyMaxMS  float64 `json:"query.latency.max.ms"`
		QueryLatencyMinMS  float64 `json:"query.latency.min.ms"`
		QueryLatencyMeanMS float64 `json:"query.latency.mean.ms"`
		Time               string  `json:"time"`
		Message            string  `json:"message"`
	}
	OnMetrics = func(Metrics)
)
