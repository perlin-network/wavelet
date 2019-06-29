---
id: ledger
title: Ledger
sidebar_label: Ledger
---

#### General Information
Ledger ([ledger.go](https://github.com/perlin-network/wavelet/blob/master/ledger.go)) is the module which holds [distributed ledger](https://en.wikipedia.org/wiki/Distributed_ledger) implementation used in the Wavelet.

[Ledger](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L45) is main structure which holds all required information and implements all the functionality - gossiping, querying, nop transaction broadcasting etc.
All public/exported API is concurrently safe.

`Ledger` is core of the `wavelet` which contains all the building blocks - [graph](link to graph documentation), [accounts](link to accounts info), [gossiper](), two [snowball]() instances and orchestrates their work into single system.

#### Ledger creation
`Ledger` starts with creating [new ledger](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L73) where all parts are instantiated. Main thing here is that if there is no information about previous rounds stored in the database, genesis inception should be performed.
After that 3 separate background processes will be start
- [syncing loop](#ledgersyncing)
- [consensus loop](#ledgerconsensusachievement)
- [transaction rate limiting token issuing](#incomingtransactionratelimiting)

#### Adding Transaction
First of all transaction added to the graph (see graph transaction adding). If there is an error and its not one indicating that transaction already exists ledger returns error. In case of already existing transaction error is ignored.
In case of no error 

#### Ledger Syncing

#### Ledger Consensus Achievement

#### Incoming Transaction Rate Limiting
Incoming transaction rate limiting consists of 2 parts:
- issuing "send tokens" (empty values) at a rate 1 token per 1ms (with [buffer](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L129))
- consuming token for each successfully added transaction to the ledger

Note: if there are no tokens while adding transaction to the ledger then nothing happens. I.e. rate limiting affects only sending transaction from current node since token availability checked only in the [API](https://github.com/perlin-network/wavelet/blob/master/api/mod.go#L204).