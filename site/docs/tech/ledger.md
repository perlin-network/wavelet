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
First of all transaction will be added to the graph (see graph transaction adding). If there is an error and its not one indicating that transaction already exists ledger returns error. In case of already existing transaction error is ignored.
In case of no error [send token will be taken](#incomingtransactionratelimiting) even if there is no one, transaction will be [added to gossiper](#gossiping). If thats not a "nop" transaction current time will be recorded to determine when most recent non nop transaction was received (see [broadcasting nop transactions](#broadcastingnoptransactions)). If transaction added, was created on that node and if there is no transaction selected by snowball yet, nop broadcasting may be started.

#### Ledger Syncing
1. [Syncing](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L679) starts with sending concurrent requests for latest round to `snowball k` numbers of closes peers. Round received will participate in voting in case:
    * if round id is not [zero round id](https://github.com/perlin-network/wavelet/blob/master/common.go#L45) and both start and end transaction ids aren't [zero transaction id](https://github.com/perlin-network/wavelet/blob/master/common.go#L44)
    * if depth of end transaction of received round is greater than depth of start transaction
    * if index of received round is greater or equal to index of local round by [exact](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L55) number of rounds 
2. If snowball instance dedicated to syncing not came to conclusion on some round, process of collection latest rounds from peers will continue with [exponential delay](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L762)
3. Index of snowball decided round should be greater or equal to index of local round by [exact](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L55) number of rounds, otherwise syncing process will be started over
4. Before starting with actual syncing with new decided round, all [consensus related processes](#ledgerconsensusachievement) should be stopped; they will be restarted after syncing is finished
5. After its determined that we out of sync, sync requests with current round index are sent to `snowball k` number of closest peers and responses with latest rounds and chunks of AVL account tree received
6. Responses from peers are grouped by index of latest rounds received and the one, received from [most](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L887) peers, will be chosen; if there is no such, syncing process will be started over
7. Then for each consecutive chunk of AVL tree (defined by its index), which were received from majority of peers, grouped by their checksums. The ones received from [majority](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L947) of peers again, will be applied.   
8. When chunks which should be applied, selected, they downloaded asynchronously and applied to local AVL tree snapshot.
9. Latest round received is saved and graph updated with new root and cleaned from previous round (see graph pruning).

In order to save bandwidth syncing process is made with use of grpc channels. So checksums of chunks sent first in headers, which makes decision process based on majority selection being possible to work without downloading actual chunks. And only chunks which are sent by majority are downloaded and then applied. 

#### Ledger Consensus Achievement
Ledger consensus consists of two separate processes running in parallel:
- [missing transactions downloading](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L262)
- [round finalization](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L263)

##### Pulling Missing Transactions
[Pulling missing transactions](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L310) takes all transaction in graph's missing index, if there are none there is delay for 1s before next try. If there are more than 256 of those, only 256 will be taken.
Closest peers of the ledger is taken and one is randomly selected, which will receive download request. If there are no closest peers, 1s delay will be applied before trying again. Then list of missing transaction ids is randomised, request formed and sent to selected peer. All transactions, which were successfully received from the peer will be added to the ledger.  

##### Round Finalization
[Round finalization](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L397) is an infinite loop, in which ledger first finds snowball preferred critical transaction (consider this one as locally preferred), then sends it to peers and with use of snowball decides which transaction will be used as end of consensus round.
As this transaction is found, all transactions within the round will be applied to the ledger state and graph will be cleaned from the previous round.

##### Loop Algorithm
1. Select closest peers. If there are less than [certain amount](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L47) of those, continue to next finalization loop iteration in 1s
2. Calculate difficulty based on current round
3. Take snowball preferred critical transaction
    * if there is none, take eligible critical transaction from the graph
    * if there is no eligible critical transaction in the graph, try to [broadcast nop](#broadcastingnoptransactions)
        * if there was no nop transaction created, continue to next finalization loop iteration with 1ms delay
        * if there was one created and it is critical, then continue to next finalization loop iteration, if it was not critical then wait for 500µs delay before proceeding with next loop iteration 
    * if there is eligible critical transaction in the graph then [collapse transactions](#collapsingtransactions) using it, create new round using end of previous one as a start and newly created transaction as an end; and make snowball prefer this round
    * continue to next finalization loop iteration
4. If there is snowball preferred transaction, switch nop broadcasting off and collect votes from peers for next round
    * until snowball reached decision on certain transaction to finalize current round there will be running [certain amount](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L465) of concurrent querying processes, sending requests to each peer (on each iteration peers will be selected on random from the closest peers)
    * each process will send same request consecutively to each selected peer with 3s timeout, within request next round index (current round index + 1) will be send and round for given index will be received
    * in case of error during request, querying process will switch to next peer
    * if round received cannot be unmarshaled, vote will be [counted](#collectingvotes) with empty preferred round and querying process will switch to next peer
    * if round id is [zero round id](https://github.com/perlin-network/wavelet/blob/master/common.go#L45) or either start or end transaction id is [zero transaction id](https://github.com/perlin-network/wavelet/blob/master/common.go#L44) vote will be counted with empty preferred round and querying process will switch to next peer
    * if depth of end transaction of received round is less or equal than depth of start transaction or id of start transaction of received round does not equal to id of end transaction of current round, response will be ignored and querying process will switch to next peer
    * if index of received round does not equal to index of local round response will be ignored and querying process will switch to next peer, but if its bigger by [exact number](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L55) of rounds, this round should be considered in [syncing process](#ledgersyncing)
    * start and end transactions of received round will be added to the ledger, in case of any error or if end transaction is not critical for current difficulty, querying process will switch to next peer
    * collapse transaction using round received, in case of error or if collapse results differ from the one within the round received querying process will switch to next peer
    * in case there were no errors so far, preferred round received will be taken into voting
5. When snowball reached consensus and all querying processes finished and snowball has new preferred round
    * collapse transaction with this round, in case of error or if collapse results differ from the one within the finalized round proceed to next finalizing loop iteration
    * preferred round will be saved (see rounds saving) and graph should be cleaned (see graph's pruning) for transaction below depth of end transaction of evicted round (see rounds eviction)
    * graph should be updated with new root - end transaction of finalized round
    * collapse transaction results should be committed to account (see account commit)

##### Collecting Votes
[Collecting votes](https://github.com/perlin-network/wavelet/blob/master/vote.go#L33) loop iterates through all received votes and takes only unique votes by voter's public key. As soon as [specified amount](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L47) of unique votes got, number of votes counted per each round and amount of stakes summed up per each voted round (if accounts stake is less then [minimum](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L77), minimum value is taken).
Then maximum number of votes for some round is found and maximum stake is found. Those values are used in order to determine "weight" of each round in a way
```
weight[round] = (number_of_votes[round] / maximum_votes) * (amount_of_stake[round] / maximum_stake)
```
First round with weight greater than or equal to [snowball alpha](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L48) is recorder to snowball.
Process of collecting votes starts over for rest of votes.

#### Gossiping
[Gossip](https://github.com/perlin-network/wavelet/blob/master/gossip.go) process groups transaction which needs to be gossiped in batches and sends them to closest peers.
Gossiper uses [debouncer](https://github.com/perlin-network/wavelet/blob/master/debounce/debounce.go#L133) which sends transaction either if chunk size reaches [limit](https://github.com/perlin-network/wavelet/blob/master/gossip.go#L53) or if timer fires.

#### Broadcasting Nop Transactions
[Nop transaction broadcasting](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L275) is performed for consensus to be achieved faster, i.e. "dummy" transactions are created and broadcasted to speed up process of finding critical transaction and round finalization.
So in order for broadcasting to work three conditions should be fulfilled:
- non nop transaction was received at least 100ms ago
- there was sent any transaction (including nop) created on the current node while snowball did not selected critical transaction
- current node's account should have enough [balance](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L74) to send nop transactions

In case of all conditions are met, new nop transaction will be created and [added](#addingtransaction) to the ledger.

#### Incoming Transaction Rate Limiting
Incoming transaction rate limiting consists of 2 parts:
- issuing "send tokens" (empty values) at a rate 1 token per 1ms (with [buffer](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L129))
- consuming token for each successfully added transaction to the ledger

Note: if there are no tokens while adding transaction to the ledger then nothing happens. I.e. rate limiting affects only sending transaction from current node since token availability checked only in the [API](https://github.com/perlin-network/wavelet/blob/master/api/mod.go#L204).

#### Collapsing Transactions
[Collapse transactions](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L1205) applies all valid transactions within given depth interval to accounts snapshot.
Since collapse transaction can be performed more than once for same transaction's interval its [result](https://github.com/perlin-network/wavelet/blob/master/ledger.go#L1179) is cached. And if there is cached collapse result for given end transaction id, its returned with no operation performed.

1. Starting from the given end transaction, while depth of the given start transaction is not reached, all transactions are recursively walked and put into the queue. I.e. first parents of the end transaction are taken, then parent of those parents are taken and so on and so forth. If there is missing transaction found, error returned
2. All transactions from the queue will be applied to accounts snapshot and validators will be [rewarded](#validatorsreward). If any error occurs, transaction will be marked as rejected, otherwise it will be considered as applied
3. If round is greater than or equal to [withdrawal rounds delay](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L81), then reward withdrawal requests will be processed if any (see processing reward withdrawal requests)
4. Collapse results will be put to cache

#### Validators Reward
Candidates for reward are taken as senders of parent transactions which selected recursively starting from parents of given transaction and up to [number](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L62) of depth levels. Sender is considered as candidate only if their stake is bigger than [minimum stake](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L77).
Then validator to receive reward is selected as one, whos relation `stake/total_candidates_stake` is bigger than `threshold`. Where `threshold` is calculated as
```
threshold = uint64_from_binary(entropy) % 0xFFFF / 0xFFFF
```
Where `entropy` is `BLAKE2B 256` hash from ids of all transaction, parents of which were taken as candidates.

If there was no validator selected this way, then its selected as last one from list of candidates. 
Fee/reward is [fixed](https://github.com/perlin-network/wavelet/blob/master/sys/const.go#L74) and its taken from transaction sender's balance and put not directly into validator's balance, but on validator's special reward balance. Which can then be withdrawn to actual balance.