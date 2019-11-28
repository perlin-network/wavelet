package wctl

// OnError called on any WS error
type OnError = func(error)

// Docs: https://wavelet.perlin.net/docs/ws

// Mod: accounts
type (
	OnBalanceUpdated    = func(BalanceUpdate)
	OnGasBalanceUpdated = func(GasBalanceUpdate)
	OnNumPagesUpdated   = func(NumPagesUpdated)
	OnStakeUpdated      = func(StakeUpdated)
	OnRewardUpdated     = func(RewardUpdated)
)

// Mod: network
type (
	OnPeerJoin  = func(PeerJoin)
	OnPeerLeave = func(PeerLeave)
)

// Mod: consensus
type (
	OnProposal  = func(Proposal)
	OnFinalized = func(Finalized)
)

// Mod: stake
type (
	OnStakeRewardValidator = func(StakeRewardValidator)
)

// Mod: contract
type (
	OnContractGas = func(ContractGas)
	OnContractLog = func(ContractLog)
)

// Mod: sync
type (
	
)

// Mod: tx
type (
	// This is at sync?
	OnTxApplied     = func(TxApplied)
	OnTxGossipError = func(TxGossipError)
	OnTxFailed      = func(TxFailed)
)

// Mod: metrics
type (
	OnMetrics = func(Metrics)
)
