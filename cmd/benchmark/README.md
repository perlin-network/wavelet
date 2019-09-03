# Benchmark

The benchmark package provides a tool to benchmark Wavelet Nodes.

There are two commands, `local`, and `remote`:
- `local` command will spawn some number of Nodes locally and spam transactions on first node.

- `remote` command will let you to run benchmark on an already-running Node, and spam transactions on the node.

<br />

You're most likely to use `remote` command as it's more flexible than `local`.
Also, you can create your own network of Nodes to your liking and spam transactions on one of the Node in the network.

## Remote

Using `remote`, you can spam transactions, contracts, and poll consensus.

You can spam two types of transactions at the same time:
- Contract creation
- Others, depending on the specified tag.

You can specify the transaction's payload, and tag.
By default, the transaction tag is `TagBatch` where the transaction consists of 40 transactions of Place Stake.

You can combine multiple flags to spam multiple transaction types at once.

Examples:
```
// Spam transaction as fast as possible
$ go run . remote 

// Spam 10 transaction per second
$ go run . remote -limit 10

// Spam custom payload transactions as fast as possible and fetch memory pages every consensus round.
$ go run . remote -poll 796396af30ed080f4cba59081998c42e3eb939b2f2b382bc063f70defd453187 -payload 796396af30ed080f4cba59081998c42e3eb939b2f2b382bc063f70defd453187000000000000000040420f0000000000000000000000000005000000636c69636b

// Spam 10 contract creation transaction per second, and don't spam the other transaction.
$ go run . remote -tx=false -contract contracts/transfer_back.wasm -limit 10
```

Refer to below for the available flags.

### Spam Transaction 

Used to spam transactions into the node.

You can customize the transaction's tag and payload by using other flags.
The default transaction is a batch transaction which consists of 40 Place Stake transactions.

The flag is `-tx=[true|false]`.

Default value is true.

So, you would pass this flag only to disable the spamming of transactions.

```
$ go run . remote -tx=false // don't spam transcations.
````

### Set Transaction Payload

Used to specify the transactions payload.
The transaction tag is defaulted to TagTransfer.

The flag is `-payload [hex-encoded payload]`.

```
$ go run . remote -tx -payload aa75c4f3283c1fd607d13f051685226f16c19da67884b0cdb5bb61cf09c1c337000000000000000040420f0000000000000000000000000005000000636c69636b00000000
```

### Poll Consensus

Used to poll the consensus, and fetch memory pages of the provided contract on every consensus round.

The flag is `-poll [hex-encoded contract ID]`.

```
$ go run . remote -poll aa75c4f3283c1fd607d13f051685226f16c19da67884b0cdb5bb61cf09c1c337
```

### Contract Transactions

Used to spam contract creation transactions.

The flag is `-contract [path to contract file]`.

```
$ go run . remote -poll contracts/chat.wasm
```

### Limit

Used to set the limit per second of the number of transactions to send to the node.

The Limit is applied to `-tx` transactions and `contract` transactions.

0 means there's no limit.

The flag is `-limit [positive integer]`.

Default value is 0.

```
// limit to 10 transactions per second
$ go run . remote -limit 10 

// no limit. spam as fast as possible.
$ go run . remote -limit 0 
```

## Local

Using `Local`, you can create a number of nodes and spam transactions into the first Node.

You can specify the the number of nodes by using `-count` flag.

```
$ go run . local -count 3
```

