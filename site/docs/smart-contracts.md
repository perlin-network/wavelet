---
id: smart-contracts
title: Smart Contracts
sidebar_label: Smart Contracts
---

A smart contract is an account that autonomously operates according to some code, instead of some designated wallet holder. A smart contract may create, send, and receive both transactions and PERLs much like any other account, though holds additional state much like any arbitrary program.

The intent of supporting smart contracts is to enable developers to write and deploy pieces of code which are verifiably and autonomously executed across a large networked cluster of untrusted computers. 

Wavelet supports deploying out smart contracts whose code is written in any language that compiles down to WebAssembly. For the moment, Wavelet smart contract development supports the Rust programming language as a first-class citizen.

In the future, additional programming languages such as C, C++, AssemblyScript, Zig, and Go (tinygo) will be officially supported. We are looking for contributors interested in maintaining independent smart contract
SDKs including the ones maintained by us at Perlin.

In this tutorial, we will look first-hand on how a simple WebAssembly (Rust) smart contract may be created, tested, and deployed on Wavelet.

## Setup

As a prerequisite, make sure [you have Rust installed](https://www.rust-lang.org/tools/install) with the WebAssembly compiler backend target installed on the Nightly channel.

To install the WebAssembly compiler backend target after installing Rust, execute the following command below and wait until it completes:

```shell
❯ rustup target add wasm32-unknown-unknown
```

Now, let's get started building our first WebAssembly smart contract.

## My First Smart Contract

Writing a smart contract is much like writing any other typical Rust application. To start off, first create a new Rust project which will house the contents of your smart contract:

```shell
❯ cargo new --lib my-first-contract; cd my-first-contract 
```

Afterwards, lets import in Wavelet's Rust smart contract SDK into our `Cargo.toml`, and turn on Link-time Optimization (LTO) by default to reduce our resulting smart contracts binary size. The larger the smart contract binary, the more expensive it will be in terms of PERLs to deploy it on Wavelet.

```toml
[profile.release]
lto = true

[dependencies]
smart-contract = "0.1.0"
smart-contract-macros = "0.1.0"
```

Open up `src/lib.rs` and paste the code below. What our first smart contract will do is that whenever the
smart contract receives any amount of PERLs from some sender, it will always send the sender back half of the PERLs
it receives.

```rust
use std::error::Error;

use smart_contract::payload::Parameters;
use smart_contract::transaction::{Transaction, Transfer};
use smart_contract_macro::smart_contract;

pub struct Contract;

#[smart_contract]
impl Contract {
    fn init(_params: &mut Parameters) -> Self {
        Self {}
    }

    fn on_money_received(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
        // Create and send transaction.
        Transfer {
            destination: params.sender,
            amount: (params.amount + 1) / 2,
            func_name: vec![],
            func_params: vec![],
        }
        .send_transaction();

        Ok(())
    }
}
```

Let's go over what the code above does.

```rust
pub struct Contract;

#[smart_contract]
impl Contract {
    fn init(_params: &mut Parameters) -> Self {
        Self{}
    }
    
    ...
}
```

What the code above does is create a new `struct Contract` with an impl block tagged with the SDK-provided `#[smart_contract]` procedural macro.

### The `init` Function

An `init` function is defined, which acts much like a constructor in other programming languages to optionally initiate memory, populate any of our `struct Contract`'s fields, or invoke any arbitrary functions.

The `init` function in particular is special, because it is called _only once_ at the very moment the smart contract is successfully spawned by some account in
Wavelet's network. The `init` function may not be manually called or executed at any other point in time.

## Invoking Smart Contract Functions

As you might have noticed, our smart contract defines a function `on_money_received`. How would we invoke this function?

Every smart contract function is invoked by creating and publishing a smart contract invocation transaction. Within your transaction, you would specify the name of the function you
 wish to invoke, and also optionally specify an arbitrary function binary payload.
 
The payload in this case is a series of bytes representative of a list of input parameters to be passed on to your smart contract function upon invocation.

Given that smart contracts are much like any other accounts, apart from providing an arbitrary function binary payload, you may also send/delegate some amount of PERLs to the smart contract as well. The PERLs
in this case would be deposited into the balance of the smart contracts account.

### Payload Format

To construct the binary payload you wish to pass on to your transaction, all you need to do is follow these simple rules.

1. List out your typed parameters. Have all integer-typed parameters be little-endian encoded into bytes, and all variable-sized arrays be length-prefixed with an unsigned little-endian 32-bit integer.
2. Concatenate the bytes of your parameters up together, and there you have it: your binary payload.

What you would then do is take your function binary payload, and create a transaction with tagged to perform a `Transfer` operation, whose operational payload
is encoded as follows:

| Field | Type |
| ----- | ---- |
| Smart Contract ID | 256-bit address of a deployed smart contract. |
| Num PERLs Sent | Unsigned 64-bit little-endian integer, representative of some amount of PERLs to be sent to a specified smart contract account. |
| Gas Limit | Unsigned 64-bit little-endian integer, representative of the maximum gas fee that may be deducted from the transaction creators account. |
| Function Name | Length-prefixed string, representative of the name of the smart contract function to be invoked. |
| Function Payload | Length-prefixed array of bytes passed as input parameters to the smart contract function to be invoked. |

In the case of wishing to invoke `on_money_received`, you would assign `on_money_received`, and set some amount of PERLs to send to the contract, and then proceed
to create and publish your transaction on Wavelet.

### Gas Limits and Fees

Now, there is no free lunch: every smart contract deployment and call costs some amount of computational resources to the network. To compensate, some lump sum amount of PERLs must be provided as a fee, which we refer to as _gas_.

More specifically, gas is a transactional fee designated to be some number of PERLs, that is computed and deducted from your balance based on the number of instructions that nodes have to execute to complete and verify your smart contract
function invocation call across the network.

Additionally, as you might have noticed from the binary payload layout format above, there exists a concept of a _gas limit_. A gas limit denotes the maximum amount of PERLs that you may propose to the network to expend on your behalf for
completing your smart contract call.

Should in amidst calling your smart contract function that an insufficient gas limit was specified (such that the mid-way through invoking your desired function you run out of gas), all changes made in-memory by the execution of your function
will be rolled back, and an amount of PERLs all the way up to the gas limit specified will be deducted from your account.

## Developing Smart Contracts

Now, let's take a step back. Noticeably, each and every smart contract function under Wavelet's Rust smart contract SDK has a
 single function input parameter `params: &mut Parameters`.
 
### Parsing Input Parameters

The `Parameters` struct is a wrapper around your smart contract function calls binary function payload; acting as a convenient API
for parsing and decoding your smart contract calls input parameters.

The `Parameters` struct additionally provides further context about the transaction calling
the smart contract function. More specifically, the `Parameters` struct provides the following information:

```rust
pub struct Parameters {
    // An ascending, incremental counter that made by used as a notion of time.
    pub round_idx: u64, 
    
    // A non-deterministic set of 32 bytes which may be used for seeding random number generators.
    pub round_id: [u8; 32], 
    
    // The ID of the smart contract function call transaction.
    pub transaction_id: [u8; 32], 
    
    // The wallet that created and published the transaction.
    pub sender: [u8; 32], 
    
    // Number of PERLs delegated to the smart contract call.
    pub amount: u64, 
    
    ...
}
``` 

The following code below demonstrates how input parameters may be decoded and read in your smart contract using the `Parameters` struct.

```rust
fn a_smart_contract_function(params: &mut Parameters) -> Result<(), Box<dyn Error>> {
    let _a: u8 = params.read(); // Read a single unsigned byte.
    let _b: i8 = params.read(); // Read a single signed byte.
    
    let _c: u16 = params.read(); // Read a single unsigned 16-bit integer.
    let _d: i16 = params.read(); // Read a single signed 16-bit integer.
    
    let _e: u32 = params.read(); // Read a single unsigned 32-bit integer.
    let _f: i32 = params.read(); // Read a single signed 32-bit integer.
        
    let _g: u64 = params.read(); // Read a single unsigned 64-bit integer.
    let _h: i64 = params.read(); // Read a single signed 64-bit integer.
        
    let _i: u128 = params.read(); // Read a single unsigned 128-bit integer.
    let _j: i128 = params.read(); // Read a single signed 128-bit integer.
    
    let _k: bool = params.read(); // Read a single byte as a boolean. 0 is false, 1 is true.
    
    let _l: String = params.read(); // Read a single string prefixed by an unsigned 32-bit integer.
    
    let _m: Vec<u8> = params.read(); // Read a vector of bytes prefixed by an unsigned 32-bit integer.
    
    let _n: [u8; 32] = params.read(); // Read exactly 32 bytes.
    let _wallet_address: [u8; 32] = params.read(); // Wallet addresses in Wavelet are 32 bytes.
    
    // Note that the `read()` function may be type-postfixed as well.
    // For example: `let wallet_address = params.read::<[u8; 32>();`
    
    Ok(())
}
```

In the case of the smart contract that we are creating, to invoke `on_money_received`, we only require knowledge of the wallet address of the user
who sent money to our smart contract, which is accessible via `params.sender`.

### Sending Transactions

Given that a smart contract is much like any other account, a smart contract may also send and submit transactions as well.

In the case of our smart contract, our intent is to send back half the PERLs of however many PERLs the smart contract receives. The core code that performs said intent is written within the `on_money_received` function like so:

```rust
fn on_money_received(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
    // Create and send transaction.
    Transfer {
        destination: params.sender,
        amount: (params.amount + 1) / 2,
        func_name: vec![],
        func_params: vec![],
    }
    .send_transaction();

    Ok(())
}
```

The smart contract SDK provides transaction types such as `Transfer` as structs, which may be
populated with a recipient and PERL amount. Calling `send_transaction` on the struct would then
have the contract send a transaction under its own account, which is to then be processed by the
network.

Other arbitrary transaction types may be sent from a smart contract as well, such as the `Contract` type which
 is utilized to spawn new smart contracts. This allows for smart contract systems which may spawn and manage other smart contracts
 for example.
 
Note that if invalid parameters are specified in a transaction sent by a smart contract, the smart contract
may still continue executing until it finishes invoking the function that you have called.
 
### Error Handling

Smart contract functions may denote successful execution by returning an `Ok(())`, or a boxed `Error` otherwise. Returning an `Error` would roll-back any changes made within a contracts in-memory state in amidst invocation.

Note however
that all instructions that were executed before an `Error` is returned would still be deducted from your balance in the form of
gas fees.

In the case of our smart contract, we simply return `Ok(())`, though may optionally return an `Error` if for example we might want
our smart contract to only process transactions that send a minimum of 1500 PERLs.

```rust
fn on_money_received(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
    if params.amount < 1500 {
        return Err("A minimum of 1500 PERLs must be sent.".into());
    }
    
    Transfer {
        destination: params.sender,
        amount: (params.amount + 1) / 2,
        func_name: vec![],
        func_params: vec![],
    }
    .send_transaction();

    Ok(())
}
```

### Debug Logging

An additional macro is provided within the Rust smart contract SDK, which is the `debug!()` macro. The
macro follows the [RFC 2361 `dbg!()` macro's](https://github.com/rust-lang/rfcs/blob/master/text/2361-dbg-macro.md) specification.

Any variable alongside its contents may easily be printed out on any of your nodes terminal using the provided `debug!()` macro.

```rust
fn test_function(params: &mut Parameters) -> Result<(), Box<dyn Error>> {
    debug!("Hello world!");
    
    Ok(())
}
```

## Deploying Smart Contracts

So there you have it; your first smart contract. Let's now compile it down into a WebAssembly binary using Rust's package manager:

```shell
❯ cargo build --release --target wasm32-unknown-unknown
```

You may then find your first WebAssembly smart contract compiled into a binary in `target/wasm32-unknown-unknown/my_first_contract.wasm`. Make sure to keep track of the file path to your contracts binary,
as we will need it later for deploying it on Wavelet.

### The `spawn` Command

In any one of your nodes terminals, to deploy your first smart contract, simply run:

```shell
❯ spawn [file path to your contract binary here]
INF Success! Your smart contracts ID: 17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560
```

Once one consensus round passes by, you may use the `find` command to confirm whether or not
your smart contract has been successfully deployed.

```json
❯ f 17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560

{
    "account": "17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560",
    "balance": 0,
    "is_contract": true,
    "nonce": 0,
    "num_pages": 18,
    "reward": 0,
    "stake": 0
}
```

You may then try send some PERLs to your smart contract and check if it works as expected.

```json
❯ p 17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560 1000
INF Success! Your payment transaction ID: 9b696a6456dd6a497226b5f0de60833bbeb451612a4a0a0a96d1f566d9383e6a
INF Deducted PERLs for invoking smart contract function.

❯ f 17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560

{
    "account": "17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560",
    "balance": 500,
    "is_contract": true,
    "nonce": 0,
    "num_pages": 18,
    "reward": 0,
    "stake": 0
}
```

### The `call` Command

In the case that you may want to specify arbitrary input parameters and execute functions from your own self-made smart contracts, you may use the `call` command in any of your nodes terminals like so:

```shell
❯ call [contract address] [amount of perls to send] [gas limit] [function name] [function payload]
```

As an example, say that you created a smart contract which exposes a function called `transfer`.
The function takes in a boolean, a 32-byte wallet address, and an unsigned 64-bit integer.

We wish to invoke the `transfer` function with the following input parameter set:
 
```shell
(true, `17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560`, 1000)
```

We would encode our set into a `[function payload]` like so:

```shell
11 H17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560 81000
```

For a more thorough understanding on how to construct the `[function payload]` field,
skim over the table below which details how parameters for the payload are fed into the terminal:

| Prefix | Parameter Type |
| -------- | ---------------- |
| 1/2/4/8 | Unsigned 8/16/32/64-bit integer |
| H | Non-length-prefixed hex-encoded bytes |
| S | Length-prefixed string |

Given this, we may now construct any arbitrary function binary payload within our terminal. The final `call` command we would execute, assuming a gas limit of 999999 PERLs, would then be:

```shell
❯ call [contract address] 0 999999 transfer 11 H17b9165d75334fafcd9b85163409deeb6bb7873218e6406677af2da1a73ee560 81000
```