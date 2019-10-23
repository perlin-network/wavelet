# REST API endpoints

Some of the endpoints are rate limited per IP Address to 1000 requests per second. If an IP Address exceeds the limit, the response would be an error with code `429` and message `Too Many Requests`.

Some of the types have constant size in bytes, as specified below:

| Type                      | Size in bytes |
|---------------------------|---------------|
| Account ID                | 32            |
| Public Key                | 32            |
| Signature                 | 64            |
| Transaction / Contract ID | 32            |
| Merkle ID                 | 16            |
| Round ID                  | 32            |

## Ledger

   Get ledger current status.

   This endpoint is rate limited.

- **URL**: `/ledger`
- **Method**: `GET`
- **URL Params**: None
- **Data Params**: None

### Success Response:

- **Code:** 200
- **Content:**

```json
{
  "public_key": "516b859770a4942c1c109d091122097f3e3975ebe0f12fcd781a17cd60ed7477",
  "address": "127.0.0.1:3000",
  "num_accounts": 3,
  "preferred_votes": 0,
  "sync_status": "Node is taking part in consensus process",
  "preferred_id": "",
  "round": {
    "merkle_root": "cd3b0df841268ab6c987a594de29ad19",
    "start_id": "0000000000000000000000000000000000000000000000000000000000000000",
    "end_id": "403517ca121f7638349cc92d654d20ac0f63d1958c897bc0cbcc2cdfe8bc74cc",
    "index": 0,
    "depth": 0,
    "difficulty": 8
  },
  "graph": {
    "num_tx": 1,
    "num_missing_tx": 0,
    "num_tx_in_store": 1,
    "num_incomplete_tx": 0,
    "height": 1
  },
  "peers": null
}
```
 
### Error Response:

- **Code:** 429 TOO MANY REQUEST
- **Content:** `Too Many Requests`

## Account

Get Account Information

- **URL**: `/accounts/:id`
- **Method**: `GET`
- **URL Params**: 
	- `id=[string]` where `id` is the hex-encoded Account ID.
- **Data Params**: None

### Success Response:

- **Code:** 200
- **Content:**
```json
{
  "public_key": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
  "balance": 10000000000000000000,
  "gas_balance": 0,
  "stake": 0,
  "reward": 5000000,
  "nonce": 1,
  "is_contract": false
}
```
 
### Error Response:

- **Reason:** Invalid Account ID size
- **Code:** 400 BAD REQUEST 
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "account ID must be 32 bytes long"
}
```

- **Code:** 400 BAD REQUEST 
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "account ID must be presented as valid hex: [...]"
}
```
 
## Send Transaction

Send Transaction
 
- **URL:** `/tx/send`
- **Method:** `POST`
- **URL Params:** None
- **Data Params:** 
```json
{
  "sender": "[hex-encoded sender ID, must be 32 bytes long]",
  "tag": "[possible values: 0 = nop, 1 = transfer, 2 = contract, 3 = stake, 4 = batch",
  "payload": "[hex-encoded payload, empty for nop]",
  "signature": "[hex-encoded edwards25519 signature, which consists of private key, nonce, tag, and payload]"
}
```
 
### Success Response:
 
- **Code:** 200
- **Content:**
```json
{
  "tx_id": "facd9c4bddc8d1080bac6d08a35cbd98ff9ef3924624d1307eced3b40d3549a0",
  "parent_ids": [
    "4a987f2371c27a23516ce8c93367826bd5a46bf815d0c19b765a5b10292e575b"
  ],
  "is_critical": false
}
```

### Error Response:
 
- **Code:** 400 BAD REQUEST
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "error adding your transaction to graph: [...]"
}
```

## Transaction List

Get Transaction List

This endpoint is rate limited.
 
- **URL**: `/tx`
 
- **Method:**: `GET`
- **URL Params:** (optional)
	- `sender=[string]` where `sender` is the hex-encoded Sender ID. Used to filter by Sender ID.
	- `creator=[string]` where `creator` is the hex-encoded Creator ID. Used to filter by Creator ID.
    * `offset=[integer]` where `offset` is page offset. Default 0.
    * `limit=[integer]` where `limit` is page limit. If 0, it'll return all transactions.

- **Data Params:** None
 
### Success Response:
 
- **Code:** 200
- **Content:**
```json
[
  {
    "id": "a91d6df9f8b680ae5bb2aa387dc2ce0aaa9e12a92ffc145ff65332bcc41d5256",
    "sender": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "creator": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
    "status": "received",
    "nonce": 0,
    "depth": 1,
    "tag": 1,
    "payload": "QABW7minzCaVIi3wXqdodbwn7G5h6OYjF8M2FXAZxAXz4AEAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
    "seed": "177e12f2fd370425e2e870d1402319918e5be35a2dd85b1277a2020a36bbdf85",
    "seed_len": 3,
    "sender_signature": "3deb68d0684b954482d948e533dbab4212123c51f0d0c4162d85d02fcca714658eb9688ff966ee9decddc7662a64d7a8702502cd8f8a2b2cb7eb096f2550a504",
    "creator_signature": "ebf1711c85def73965aeca5dc4fb071422b565dfbf19a886f57503feff7e124e2759ff435a2bd3407fd13421611e27280dfa43c5ac5788f11381775150b1dd09",
    "parents": [
      "403517ca121f7638349cc92d654d20ac0f63d1958c897bc0cbcc2cdfe8bc74cc"
    ]
  }
]
```

### Error Response:
 
- **Code:** 429 TOO MANY REQUEST
- **Content:** `Too Many Requests`

## Transaction Detail

Get Transaction Details by ID
 
- **URL:** `/tx/:id`
- **Method:** `GET`
- **URL Params:**
	- `id=[string]` where `id` is the hex-encoded Transaction ID.
- **Data Params:** None
 
### Success Response:
 
- **Code:** 200
- **Content:**
```json
{
  "id": "a91d6df9f8b680ae5bb2aa387dc2ce0aaa9e12a92ffc145ff65332bcc41d5256",
  "sender": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
  "creator": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
  "status": "received",
  "nonce": 0,
  "depth": 1,
  "tag": 1,
  "payload": "QABW7minzCaVIi3wXqdodbwn7G5h6OYjF8M2FXAZxAXz4AEAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
  "seed": "177e12f2fd370425e2e870d1402319918e5be35a2dd85b1277a2020a36bbdf85",
  "seed_len": 3,
  "sender_signature": "3deb68d0684b954482d948e533dbab4212123c51f0d0c4162d85d02fcca714658eb9688ff966ee9decddc7662a64d7a8702502cd8f8a2b2cb7eb096f2550a504",
  "creator_signature": "ebf1711c85def73965aeca5dc4fb071422b565dfbf19a886f57503feff7e124e2759ff435a2bd3407fd13421611e27280dfa43c5ac5788f11381775150b1dd09",
  "parents": [
    "403517ca121f7638349cc92d654d20ac0f63d1958c897bc0cbcc2cdfe8bc74cc"
  ]
}
```

### Error Response:

- **Code:** 400 BAD REQUEST
- **Desc:**  The Transaction ID is not a valid size. Its size must be 32 bytes
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "transaction ID must be 32 bytes long"
}
```

- **Code:** 400 BAD REQUEST
- **Desc:** Transaction ID is not a valid hex
- **Content:**
```json
{
 "status": "Bad request.",
"error": "transaction ID must be presented as valid hex: [...]"
}
```

- **Code:** 404 NOT FOUND
- **Desc:** Transaction ID does not exist
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "could not find transaction with ID [...]"
}
```

## Contract Code

   Get Contract Code By ID.
   This endpoint is rate limited.
 
- **URL:** `/contract/:id`
- **Method:** `GET`
- **URL Params:**
	- `id=[string]` where `id` is the hex-encoded Contract ID.
- **Data Params:** None

### Success Response:
 
- **Code:** 200
- **Content:**
The contract web assembly code.

### Error Response:

- **Code:** 400 BAD REQUEST
- **Desc:** The Contract ID is not a valid size. Its size must be 32 bytes
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "contract ID must be 32 bytes long"
}
```

- **Code:** 400 BAD REQUEST
- **Desc:** The contract ID is not a valid hex
- **Content:**
```json
{
 "status": "Bad request.",
"error": "contract ID must be presented as valid hex: [...]"
}
```

- **Code:** 404 NOT FOUND
- **Desc:** The Contract ID does not exist
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "could not find contract with ID [...]"
}
```

- **Code:** 429 TOO MANY REQUEST
- **Desc:** The request is rate limited
- **Content:** `Too Many Requests`

## Contract Page

   Get Contract Page

   This endpoint is rate limited.
 
- **URL:** `/contract/:id/page/:index`
- **Method:**: `GET`
- **URL Params:**
	- `id=[string]` where `id` is the hex-encoded Contract ID.
	- `page=[integer]` where `id` is the page index.
- **Data Params:** None
 
### Success Response:
 
- **Code:** 200
- **Content:**
The memory snapshot.

### Error Response:

- **Code:** 400 BAD REQUEST
- **Desc:** The Contract ID is not a valid size. Its size must be 32 bytes
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "contract ID must be 32 bytes long"
}
```

- **Code:** 400 BAD REQUEST
- **Desc:** The contract ID is not a valid hex
- **Content:**
```json
{
 "status": "Bad request.",
"error": "contract ID must be presented as valid hex: [...]"
}
```

- **Code:** 404 NOT FOUND
- **Desc:** The contract does not have any page
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "could not find any pages for contract with ID [...]"
}
```

- **Code:** 400 BAD REQUEST
- **Desc:** The page is either empty or does not exist
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "page [...] is either empty, or does not exist"
}
```

- **Code:** 400 BAD REQUEST
- **Desc:** The index does not exist
- **Content:**
```json
{
  "status": "Bad request.",
  "error": "contract with ID [...] only has [...] pages, but you requested page [...]"
}
```

- **Code:** 429 TOO MANY REQUEST
- **Desc:** The request is rate limited
- **Content:** `Too Many Requests`
