package main

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/wctl"
)

// converts a normal (func to close, error) to only an error
func addToCloser(closer *[]func(), fns ...func() (func(), error)) error {
	for _, fn := range fns {
		close, err := fn()
		if err != nil {
			return err
		}

		*closer = append(*closer, close)
	}

	return nil
}

func setEvents(c *wctl.Client) (func(), error) {
	// api/events.go

	var toClose = []func(){}
	var cleanup = func() {
		for _, f := range toClose {
			f()
		}
	}

	c.OnError = func(err error) {
<<<<<<< HEAD
		log.Node().Err(err).Msg("WS Error occurred.")
=======
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
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
	}

	toClose = append(toClose, c.AddHandler(func(ev log.MarshalableEvent) {
		switch ev.(type) {
		case *wavelet.Metrics, *wavelet.TxApplied:
			// too verbose, ignore
			return
		}

		ev.MarshalEvent(log.Node().Info())
	}))

	err := addToCloser(
		&toClose,
		c.PollNetwork,
		c.PollAccounts,
		c.PollContracts,
		c.PollTransactions,
	)

<<<<<<< HEAD
	if err != nil {
		return cleanup, err
	}

	return cleanup, nil
}
=======
	return cleanup, nil
}

func onTxApplied(u wctl.TxApplied) {
	/* Too verbose...
	logger.Info().
		Hex("tx_id", u.TxID[:]).
		Hex("sender_id", u.SenderID[:]).
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
>>>>>>> f59047e31aa71fc9fbecf1364e37a5e5641f8e01
