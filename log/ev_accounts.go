package log

// TODO

type BalanceUpdate struct {
	AccountID [32]byte `json:"account_id"`
	Balance   uint64   `json:"balance"`
}

type GasBalanceUpdate struct {
	AccountID  [32]byte `json:"account_id"`
	GasBalance uint64   `json:"gas_balance"`
}

type NumPagesUpdated struct {
	AccountID [32]byte `json:"account_id"`
	NumPages  uint64   `json:"num_pages_updated"`
}

type StakeUpdated struct {
	AccountID [32]byte `json:"account_id"`
	Stake     uint64   `json:"stake"`
}

type RewardUpdated struct {
	AccountID [32]byte `json:"account_id"`
	Reward    uint64   `json:"reward"`
}
