package main

import (
	"errors"

	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/wctl"
)

// list of functions to defer/close
var toClose []func()

// converts a normal (func to close, error) to only an error
func addToCloser(f func(), err error) error {
	toClose = append(toClose, f)
	return err
}

// clean up all functions
func cleanUp() {
	for _, f := range toClose {
		f()
	}
}

func setEvents(c *wctl.Client) error {
	c.OnError = onError

	c.OnPeerJoin = onPeerJoin
	c.OnPeerLeave = onPeerLeave
	if err := addToCloser(c.PollNetwork()); err != nil {
		return err
	}

	c.OnBalanceUpdated = onBalanceUpdate
	c.OnStakeUpdated = onStakeUpdated
	c.OnRewardUpdated = onRewardUpdate
	if err := addToCloser(c.PollAccounts()); err != nil {
		return err
	}

	c.OnRoundEnd = onRoundEnd
	c.OnPrune = onPrune
	if err := addToCloser(c.PollConsensus()); err != nil {
		return err
	}

	c.OnContractGas = onContractGas
	c.OnContractLog = onContractLog
	if err := addToCloser(c.PollContracts()); err != nil {
		return err
	}

	c.OnTxApplied = onTxApplied
	c.OnTxGossipError = onTxGossipError
	c.OnTxFailed = onTxFailed
	if err := addToCloser(c.PollTransactions()); err != nil {
		return err
	}

	c.OnStakeRewardValidator = onStakeRewardValidator
	if err := addToCloser(c.PollStake()); err != nil {
		return err
	}

	return nil
}

func onError(err error) {
	logger := log.Node()
	logger.Err(err).Msg("WS Error occured.")
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

func onRoundEnd(u wctl.RoundEnd) {
	logger.Info().
		Uint64("num_applied_tx", u.NumAppliedTx).
		Uint64("num_rejected_tx", u.NumRejectedTx).
		Uint64("num_ignored_tx", u.NumIgnoredTx).
		Uint64("old_round", u.OldRound).
		Uint64("new_round", u.NewRound).
		Uint64("old_difficulty", u.OldDifficulty).
		Uint64("new_difficulty", u.NewDifficulty).
		Hex("new_root", u.NewRoot[:]).
		Hex("old_root", u.OldRoot[:]).
		Hex("new_merkle_root", u.NewMerkleRoot[:]).
		Hex("old_merkle_root", u.OldMerkleRoot[:]).
		Int64("round_depth", u.RoundDepth).
		Msg("Round ended: " + u.Message)
}

func onPrune(u wctl.Prune) {
	logger.Info().
		Uint64("num_tx", u.NumTx).
		Hex("current_round_id", u.CurrentRoundID[:]).
		Hex("pruned_round_id", u.PrunedRoundID[:]).
		Msg("Pruned: " + u.Message)
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
		Msg("Peer has joined.")
}
