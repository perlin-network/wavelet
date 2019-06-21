---
id: governance
title: Governance
sidebar_label: Governance
---

Governance over the validation and finalization of transactions in Wavelet is established by a reputation
system, where each and every account hoists a reputation score based on some amount of PERLs they have
staked into the network.

The larger the reputation of a given particular account, the more influence/voting power they may have in affecting
the finality of transactions across the network.

We consider nodes whose accounts have greater influence over the finality of transactions in Wavelet over other nodes
to be _validators_. As a great majority of finality across the network is thus managed by the validators
that have the highest stakes within Wavelet's network, an incentive mechanism needs to be introduced to incentivize
validators to want to keep the network safe and correct.
 
The incentive scheme chosen is this case, is that for every transaction created and broadcasted by any
arbitrary account, transaction fees are charged.

These transaction fees  are distributed as rewards to Wavelet's validators, based on the frequency/amount of activity each
individual validator partakes in keeping Wavelet safe and correct.

One striking bit to consider is that rewards recorded on an account is completely different from the balance recorded on an account.
There exists a process which a validator must partake to convert rewards into PERLs that may then be deposited into their accounts balance.

## Placing Stake

In order to become a validator, you must stake/expend a minimum amount of PERLs into the network. The exact minimum is defined within code, and is tentative and therefore
may be changed based on community discussions at a later date. To see the current minimum stake, [click here](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L77).

Note that the conversion rate between PERLs and stake is 1:1. The larger the amount of stake you have deposited into your account, the more influence/voting power you will have over
other nodes when within the entire network.

To deposit a stake of PERLs into the network, on your nodes terminal, enter:

```shell
❯ ps [amount of PERLs to stake]
INF Success! Your stake placement transaction ID: <..>
``` 

After a single consensus round finalizes, you may then check if the amount of PERLs was properly deducted from your account,
and deposited into your stake by using the `find` command.

```json
❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 10000000000000000000,
    "is_contract": false,
    "nonce": 0,
    "num_pages": 0,
    "reward": 0,
    "stake": 0
}

❯ ps 100
INF Success! Your stake placement transaction ID: <..>
INF Finalized consensus round, and initialized a new round.

❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 9999999999999999900,
    "is_contract": false,
    "nonce": 2,
    "num_pages": 0,
    "reward": 0,
    "stake": 100
}
```

## Withdrawing Stake

Should you wish to withdraw and convert your stake into PERLs, withdraw from being a validator, or diminish your voting power over the network,
you may then execute the following on your nodes terminal:

```shell
❯ ws [amount of stake to withdraw]
INF Success! Your stake withdrawal transaction ID: <..>
``` 

After a single consensus round finalizes, you may then check if the amount of PERLs was properly deducted from your stake,
and deposited into your balance by using the `find` command.

```json
❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 9999999999999999900,
    "is_contract": false,
    "nonce": 2,
    "num_pages": 0,
    "reward": 0,
    "stake": 100
}

❯ ws 100
INF Success! Your stake withdrawal transaction ID: <..>
INF Finalized consensus round, and initialized a new round.

❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 10000000000000000000,
    "is_contract": false,
    "nonce": 4,
    "num_pages": 0,
    "reward": 0,
    "stake": 0
}
```

## Fees and Rewards

As a validator, having more weight in assisting with the settlement of transactions naturally attracts the attention of both potentially
good or bad actors within the Wavelet network. In compensation, rewards are dispersed to validators in the form of transaction fees deducted on
each and every transaction finalized within the network.

There is one big challenge however in figuring out how to disperse rewards fairly over a cluster of untrusted machines:
how do we know how much effort a validator has put into validating and protecting the network in comparison to other validators?

Wavelet incentivizes and judges the efforts of a validator fairly by dispersing awards only to validators that participate
in the network by building transactions on top of other nodes transactions.

To illustrate how this system works, denote a transaction $\text{tx}$ that has just been finalized. Then:

1. Let $\text{eligible} = \{ \text{tx'.sender} \neq \text{tx.sender}\ \mid \text{tx'} \in \text{tx.ancestors} \}$. If $|\text{eligible}| = 0$, terminate the procedure.
2. Let $\text{validators} = \{ \text{tx'.sender} \mid \text{tx'} \in \text{eligible} \}$. Concatenate and hash together all of $\{ \text{tx'.id} \mid \text{tx'} \in \text{eligible} \}$ using $H$ to construct a checksum that acts as an entropy source $E$.
3. Let $X'$ be some threshold value that is the last 16 bits of $E$, and denote a random variable $X$ which models a weighted uniform distribution over the set of all candidate validators. We then select a validator $v \in \text{validators}$ that has a weight $X \geq X'$ as the transaction fee reward recipient.
4. Transfer some amount of transaction fees from $\text{tx.sender}$'s balance to the intended
   rewardee's $v$ reward balance.
   
Such a system incentivizes validators to actively create and broadcast transactions given that rewards are dispersed to those who build transactions on top of other transactions.

## Withdrawing Rewards

After accumulating a minimum amount of reward as a validator, you may convert your reward into PERLs
that may be expended for whatever reason you desire. The exact minimum is defined within code, 
and is tentative and therefore may be changed based on community discussions at a later date.
To see the current minimum reward that may be withdrawn, [click here](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L79).

Unlike withdrawing stake, withdrawing rewards is as delayed process. After withdrawing some amount of reward, only after a certain number
of consensus rounds pass by will your reward convert into PERLs that you may expend. The exact number of rounds is defined within code, and is
tentative and therefore may be changed based on community discussions at a later date. To see the current number of consensus rounds
that must pass before your reward withdrawal request is completed, [click here](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L81).

Note that once a reward withdrawal request is submitted, it may not be canceled. To initiate your reward withdrawal request, in your nodes terminal, enter:

```go
❯ wr [amount of reward to withdraw into PERLs]
INF Success! Your reward withdrawal transaction ID: <..>
```

After a single consensus round finalizes, you may then check if the amount of reward you specified is withdrawn from your account by using the `find` command.

```json
❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 10000000000000000000,
    "is_contract": false,
    "nonce": 4,
    "num_pages": 0,
    "reward": 50000,
    "stake": 0
}

❯ wr 100
INF Success! Your reward withdrawal transaction ID: <..>
INF Finalized consensus round, and initialized a new round.

❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 10000000000000000000,
    "is_contract": false,
    "nonce": 6,
    "num_pages": 0,
    "reward": 49900,
    "stake": 0
}

* After some number of consensus rounds...

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 10000000000000000100,
    "is_contract": false,
    "nonce": 6,
    "num_pages": 0,
    "reward": 49900,
    "stake": 0
}

```