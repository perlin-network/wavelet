---
id: setup
title: Getting started
sidebar_label: Quickstart
---

To get started running your own personal Wavelet testnet, download one of the
pre-built binaries [here](https://github.com/perlin-network/wavelet/releases).

Wavelet currently supports **Windows**, **Linux**, and **Mac OSX**. However, extensive testing was done primarily on
Linux. In the case that any errors arise, please open a Github issue with the error you are experiencing.

## Quickstart

Run the following commands in 3 separate terminal instances. Wavelet requires each node to be connected to atleast
2 other nodes at all times before transactions may start being made.


```shell-session
[Terminal 1] ❯ ./wavelet --port 3000 --api.port 9000 --wallet config/wallet.txt
[Terminal 2] ❯ ./wavelet --port 3001 --api.port 9001 --wallet config/wallet2.txt 127.0.0.1:3000
[Terminal 3] ❯ ./wavelet --port 3002 --api.port 9002 --wallet config/wallet3.txt 127.0.0.1:3000
```

Running the commands above will spawn three Wavelet nodes listening on ports **3000**, **3001**, and **3002**, with HTTP APIs hosted
on ports **9000**, **9001**, and **9002**.

### Bootstrap Nodes

Node 2 and Node 3 discover and interconnect with one another by designating Node 1 as a bootstrap node. Additional bootstrap nodes may be specified by providing their addresses in a space-delimited list like
so:

```shell-session
❯ ./wavelet --port 3000 --api.port 9000 --wallet config/wallet.txt 127.0.0.1:3001 127.0.0.1:3002
```

By default, nodes will persist all transactional and state data in-memory, such that nodes lose all data the very moment they
are shut down. A database path might be provided using the `--db.path [directory path]` flag to persist all data on-disk.

If everything runs properly, you should see this in Terminal 1:

```shell
❯ ./wavelet --port 3000 --api.port 9000 --wallet config/wallet.txt
INF Listening for peers. addr: 127.0.0.1:3000
INF Wallet loaded. privateKey: [private key] publicKey: [public key]
INF Started HTTP API server. port: 9000
```

### Wallet Management

Should the `--wallet [wallet path]` flag not be specified, a new wallet will randomly be generated. In the case that the default wallets are specified for each node, the wallet addresses of each individual node are:


| Node 	| Wallet address 	|
|------	|------------------------------------------------------------------	|
| 1 	| 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405 	|
| 2 	| 696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a 	|
| 3 	| f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467 	|

### Setting Up Genesis

If the default wallets are used, Node 1 and Node 2 by default should have a significant amount of PERLs in their wallet; with Node
3 having zero PERLs.

These initially minted PERLs are specified through a genesis file that must be equivalent across
all nodes within the network. The genesis file is in JSON format, and specifies the initial balance, reward, and stake each individual
node has at the genesis of the network.

**PERLs** are the cryptocurrency at the hearth of Wavelet's security, safety, and economy. PERLs may be
spent for spawning/invoking smart contracts, sent/received as an asset, and otherwise earned by assisting the network with
validating and processing transactions. 

## My First Transaction

Now, let's get to making your first transaction. In Node 1's terminal, type the following and press [Enter]:

```shell
❯ p f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467 128
```

The command above will send Node 3 **128 PERLs**. And there you have it; your first transaction.

```shell
INF Finalized consensus round, and initialized a new round.
```

The very moment you see the above line in Node 1's terminal, your transaction is finalized with its intent of transferring
PERLs from Node 1 to Node 3 being applied, finalized, and replicated across all the nodes in your local network.

### The `find` Command

To check that your transaction has been applied and finalized successfully, you may execute the following command on any of
your nodes:

```json
❯ f f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467

{
    "account": "f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467",
    "balance": 128,
    "is_contract": false,
    "nonce": 0,
    "num_pages": 0,
    "reward": 0,
    "stake": 0
}

❯ f 400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405

{
    "account": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "balance": 9999999999999999872,
    "is_contract": false,
    "nonce": 0,
    "num_pages": 0,
    "reward": 0,
    "stake": 0
}
```

The `f` command is short for the `find [wallet address/smart contract address/transaction id]` command, which searches
for and prints out the details of any wallet/smart contract/transaction you desire.