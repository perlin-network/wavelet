package main

import (
	"errors"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/wctl"
)

// converts a normal (func to close, error) to only an error
func addToCloser(toClose *[]func()) func(f func(), err error) error {
	return func(f func(), err error) error {
		if f != nil {
			*toClose = append(*toClose, f)
		}
		return err
	}
}

func setEvents(c *wctl.Client) (func(), error) {
	var toClose []func()
	cleanup := func() {
		for _, f := range toClose {
			f()
		}
	}

	logger := log.Node()
	c.OnError = func(err error) {
		logger.Err(err).Msg("WS Error occurred.")
	}

	c.OnPeerJoin = onPeerJoin
	c.OnPeerLeave = onPeerLeave
	if err := addToCloser(&toClose)(c.PollNetwork()); err != nil {
		return cleanup, err
	}

	c.OnBalanceUpdated = onBalanceUpdate
	c.OnGasBalanceUpdated = onGasBalanceUpdated
	c.OnStakeUpdated = onStakeUpdated
	c.OnRewardUpdated = onRewardUpdate
	if err := addToCloser(&toClose)(c.PollAccounts()); err != nil {
		return cleanup, err
	}

	c.OnProposal = onProposal
	c.OnFinalized = onFinalized

	c.OnContractGas = onContractGas
	c.OnContractLog = onContractLog
	if err := addToCloser(&toClose)(c.PollContracts()); err != nil {
		return cleanup, err
	}

	c.OnTxApplied = onTxApplied
	c.OnTxGossipError = onTxGossipError
	c.OnTxFailed = onTxFailed
	if err := addToCloser(&toClose)(c.PollTransactions()); err != nil {
		return cleanup, err
	}

	c.OnStakeRewardValidator = onStakeRewardValidator
	if err := addToCloser(&toClose)(c.PollStake()); err != nil {
		return cleanup, err
	}

	return cleanup, nil
}

func onStakeRewardValidator(r wctl.StakeRewardValidator) {
	logger.Info().
		Hex("creator", r.Creator[:]).
		Hex("recipient", r.Recipient[:]).
		Hex("creator_tx_id", r.CreatorTxID[:]).
		Hex("rewardee_tx_id", r.RewardeeTxID[:]).
		Hex("entropy", r.Entropy[:]).
		Float64("accuracy", r.Accuracy).
		Float64("threshold", r.Threshold).
		Msg(r.Message)
}

func onTxApplied(u wctl.TxApplied) {
	/* Too verbose...
	logger.Info().
		Hex("tx_id", u.TxID[:]).
		Hex("sender_id", u.SenderID[:]).
		Hex("creator_id", u.CreatorID[:]).
		Uint64("depth", u.Depth).
		Uint8("tag", u.Tag).
		Msg("Transaction applied.")
	*/
}

func onTxGossipError(u wctl.TxGossipError) {
	logger.Err(errors.New(u.Error)).
		Msg(u.Message)
}

func onTxFailed(u wctl.TxFailed) {
	logger.Err(errors.New(u.Error)).
		Hex("tx_id", u.TxID[:]).
		Hex("sender_id", u.SenderID[:]).
		Hex("creator_id", u.CreatorID[:]).
		Uint64("depth", u.Depth).
		Uint8("tag", u.Tag).
		Msg("Transaction failed.")
}

func onContractGas(u wctl.ContractGas) {
	logger.Info().
		Hex("sender_id", u.SenderID[:]).
		Hex("contract_id", u.ContractID[:]).
		Uint64("gas", u.Gas).
		Uint64("gas_limit", u.GasLimit).
		Msg(u.Message)
}

func onContractLog(u wctl.ContractLog) {
	logger.Info().
		Hex("contract_id", u.ContractID[:]).
		Msg(u.Message)
}

func onProposal(u wctl.Proposal) {
	logger.Info().
		Hex("block_id", u.BlockID[:]).
		Uint64("block_index", u.BlockIndex).
		Uint64("num_transactions", u.NumTxs).
		Msg(u.Message)
}

func onFinalized(u wctl.Finalized) {
	logger.Info().
		Hex("block_id", u.BlockID[:]).
		Uint64("block_index", u.BlockHeight).
		Int("num_applied_tx", u.NumApplied).
		Int("num_rejected_tx", u.NumRejected).
		Int("num_pruned_tx", u.NumPruned).
		Msg(u.Message)
}

func onGasBalanceUpdated(u wctl.GasBalanceUpdate) {
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Uint64("gas_amount", u.GasBalance).
		Msg("Gas balance updated.")
}

func onBalanceUpdate(u wctl.BalanceUpdate) {
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Uint64("amount", u.Balance).
		Msg("Balance updated.")
}

func onStakeUpdated(u wctl.StakeUpdated) {
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Uint64("stake", u.Stake).
		Msg("Stake updated.")
}

func onRewardUpdate(u wctl.RewardUpdated) {
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Uint64("reward", u.Reward).
		Msg("Reward updated.")
}

func onPeerJoin(u wctl.PeerJoin) {
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Str("address", u.Address).
		Msg("Peer has joined.")
}

func onPeerLeave(u wctl.PeerLeave) {
	logger.Info().
		Hex("public_key", u.AccountID[:]).
		Str("address", u.Address).
		Msg("Peer has left.")
}
