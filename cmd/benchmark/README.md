# Benchmark

The benchmark package provides a tool to benchmark Wavelet Nodes.

There are two commands, `local`, and `remote`:
- `local` command will spawn some number of Nodes locally and spam transactions on first node.

- `remote` command will let you to run benchmark on an already-running Node, and spam transactions on the node.

<br />

You're most likely to use `remote` command as it's more flexible than `local`.
Also, you can create your own network of Nodes to your liking and spam transactions on one of the Node in the network.

## Remote

Using `remote`, you can spam a batch of stake transactions, custom payload transactions, contract creation transactions,
and poll consensus.

You can combine multiple flags to spam multiple transaction types at once.

### Batch of Stake Transactions

The flag is `-tx`.

```
$ go run . remote -tx
````

This will spam transactions with each transaction is a batch of 40 Place Stake transactions.

### Payload Transactions

The flag is `-payload [hex-encoded payload]`.

```
$ go run . remote -tx -payload aa75c4f3283c1fd607d13f051685226f16c19da67884b0cdb5bb61cf09c1c337000000000000000040420f0000000000000000000000000005000000636c69636b00000000
```

This will spam transactions with the custom payload.

### Poll Consensus

The flag is `-poll [hex-encoded contract ID]`.

```
$ go run . remote -poll aa75c4f3283c1fd607d13f051685226f16c19da67884b0cdb5bb61cf09c1c337
```

This will poll the consensus, and fetch memory pages of the contract on every consensus round.

### Contract Transactions

The flag is `-contract [path to contract file]`.

```
$ go run . remote -poll contracts/chat.wasm
```

This will spam contract transactions.

## Local

Using `Local`, you can create a number of nodes and spam transactions into the first Node.

You can specify the the number of nodes by using `-count` flag.

```
$ go run . local -count 3
```

