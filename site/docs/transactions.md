---
id: transactions
title: Transactions
---

A transaction is an atomic set of changes to be applied to Wavelet's network-wide replicated state;
proposed by some arbitrary account that is commonly referred to as the _transaction's creator_.

These changes are denoted on the transaction through the specification of two entries: a _tag_, and a _payload_.

Functionally, one may think of a tag as an identifier for some form of operation to be applied to the ledgers state, with the payload providing
further detail on how the operation designated by the tag should be performed.

More importantly, there only exists a limited number of tags such that there are constraints in how one may
change or modify Wavelet's state. Each individual tag holds further constraints on how the payload may be
structured/formed.

## Tags and Payloads

Internally, a tag is represented by a single byte, with the payload being an unsigned little-endian 32-bit
integer-prefixed variable length byte array.

The following table below denotes all the available tags and their operations in Wavelet:

| Tag | Binary | Description |
| --- | --------------------- | ----------- |
| `Nop` | 0x00 | No-op. `Nop` transactions must have an empty payload. |
| `Transfer` | 0x01 | Send PERLs to an arbitrary account, or invoke a smart contract function with a specified gas limit and a binary payload. For information on how `Transfer` transaction payloads are constructed, [click here](#the-transfer-transaction). |
| `Stake` | 0x02 | Place/withdraw stakes of virtual currency to become/withdraw from being a validator, or convert rewards into PERLs which were earned from participating in the network as a validator. For more information on how `Stake` transaction payloads are constructed, [click here](#the-stake-transaction). |
| `Contract` | 0x03 | Spawn and initialize a new smart contract with a specified gas limit and a binary payload. For information on how `Contract` transaction payloads are constructed, [click here](#the-contract-transaction). |
| `Batch` | 0x04 | Atomically apply a series of operations by specifying a list of tags and payloads. For information on how `Batch` transaction payloads are constructed, [click here](#the-batch-transaction). |

## Identities and Signatures

All transactions are fingerprinted by the checksum of their contents. The checksum, otherwise known as the transaction's ID, is generated using the BLAKE2b 256-bit
hash function.

All account IDs within Wavelet are 256-bit public keys of an Ed25519 keypair, with all cryptographic signatures made by Wavelet accounts complying with the Ed25519
cryptographic signature scheme standard.

## Sender and Creator

A transaction lists/references two account ID's: a _transaction creator_, and a _transaction sender_. All operations/changes denoted within a transaction are to be applied with respect to its creators account.

A creator may optionally delegate the responsibilities of broadcasting and spreading their transaction to another account, being a transaction sender.

Upon the creation of a transaction, the transaction creator would sign the tag, payload, and nonce of the transaction and pass it over to another account that
would play the role of being the transactions sender. The sender would then assign consensus-related information to the transaction, sign the entirety of
the transaction, and broadcast it out to the network to be verified and finalized by other Wavelet nodes.

## Replay Attacks

A nonce is associated to each and every Wavelet account. A nonce is an incremental, ascending counter that gets incremented every single time a transaction
that was created by some given account gets finalized and apply to the ledgers state.

The nonce is used to prevent replay attacks, where after an account creates a transaction, there may exist a possibility that several nodes may attempt
to re-sign the transaction such that the transaction may operate and be applied on the ledger indefinite amounts of times.

By attaching a nonce counter, once a single instance of some accounts transaction gets finalized, no other node may re-sign and re-broadcast the transaction
to cause a replay attack.

## Binary Format

Transactions are encoded using a simple binary encoding scheme, where all integers are little-endian encoded, and all variable-sized arrays are
length-prefixed with an unsigned 32-bit little-endian integer.

The current binary format of a Wavelet transaction is denoted as follows:

| Field | Type |
| ----- | ---- |
| Flag | A single byte that is 1 if the Creator Account ID is the same as the Sender Account ID, and is 0 otherwise. |
| Sender Account ID | 256-bit wallet address/public key. | 
| Creator Account ID | 256-bit wallet address/public key. | 
| Nonce | Latest nonce value of the creators account, denoted as an unsigned 64-bit little-endian integer. | 
| Parent IDs | Length-prefixed array of 256-bit transaction IDs; assigned by the transactions sender. |
| Parent Seeds | Array of 256-bit transaction seeds, with the same length as the Parent IDs field and therefore not length-prefixed; must correspond to the transactions specified by Parent IDs. |
| Depth | Unsigned 64-bit little-endian integer; assigned by the transactions sender. |
| Tag | 8-bit integer (byte) identifying the transactions operation. |
| Payload | Length-prefixed array of bytes providing further details of the operation invoked under the transactions designated tag. |
| Sender Signature | Ed25519 signature of the contents of the entire transaction; assigned by the transactions sender. |
| Creator Signature | Ed25519 signature of the tag, nonce, and payload concatenated together. |

As a space-saving optimization, should the sender and creator of the transaction be the exact same
account, the creator's account ID and signature is omitted when encoding the transaction into binary.
The flag byte is responsible for recording whether or not the sender and creator of the transaction is
the same.

## Payload Binary Formats

Let's go over a few of the different payload formats for certain tag types.

### The `Transfer` Transaction

The intent of a `Transfer` transaction is to either send PERLs to a specified recipient account, or to otherwise invoke a function from a smart
contract.

A `Transfer` transaction is structured, assuming the same binary encoding scheme for transactions in general, as follows:

| Field | Type |
| ----- | ---- |
| Recipient Account ID | 256-bit wallet/smart contract account address. |
| Num PERLs Sent | Unsigned 64-bit little-endian integer, representative of some amount of PERLs to be sent to the designated recipient. |
| Gas Limit | Unsigned 64-bit little-endian integer, representative of the maximum gas fee that may be deducted from the transaction creators account. |
| Gas Deposit | Unsigned 64-bit little-endian integer, representative of some amount of gas fees to deposit into the smart contract. |
| Function Name | Length-prefixed string, representative of the name of the smart contract function to be invoked. |
| Function Payload | Length-prefixed array of bytes passed as input parameters to the smart contract function to be invoked. |

For more information on how to invoke a function from a smart contract, or for what the Gas Limit, Function Name, or Function Payload
 represents within a `Transfer` transaction, [click here](smart-contracts.md#invoking-smart-contract-functions).
 
### The `Stake` Transaction

The intent of a `Stake` transaction is to either:

1. place a stake of virtual currency to register yourself as a validator or otherwise have more voting
power within the network,
2. withdraw existing stakes of virtual currency to withdraw yourself from being a validator, or
3. to convert your earned rewards
into PERLs which were earned from your work in validating and protecting the Wavelet network as a validator.

A `Stake` transaction is structured, assuming the same binary encoding scheme for transactions in general, as follows:

| Field | Type |
| ----- | ---- |
| Operation | A single byte, where 0x00 = `Withdraw Stake`, 0x01 = `Place Stake`, and 0x02 = `Withdraw Rewards`. |
| Amount | An unsigned little-endian 64-bit integer denoting some amount of PERLs to either place as stake, withdraw from stake, or withdraw from available rewards. |

### The `Contract` Transaction

The intent of a `Contract` transaction is to spawn a new smart contract, whose ID is the transactions ID. Code for the smart contract is provided
in the transaction as the functions payload.

More specifically, a `Contract` transaction is structured, assuming the same binary encoding scheme for transactions in general, as follows:

| Field | Type |
| ----- | ---- |
| Gas Limit | Unsigned 64-bit little-endian integer, representative of the maximum gas fee that may be deducted from the transaction creators account. |
| Gas Deposit | Unsigned 64-bit little-endian integer, representative of some amount of gas fees to deposit into the smart contract. |
| Payload | Length-prefixed array of bytes passed as input parameters to the smart contracts `init` function. |
| Code | Non-length-prefixed array of bytes representative of the smart contracts code. |

For more information on how to deploy a smart contract, [click here](smart-contracts.md#deploying-smart-contracts).

### The `Batch` Transaction

The intent of a `Batch` transaction is to atomically apply a batch of operations within a single transaction.

The payload of a `Batch` transaction is structed as a length-prefixed variable-length list of entries comprised of both tags and payloads, with the prefixed length encoded as
a single unsigned byte.