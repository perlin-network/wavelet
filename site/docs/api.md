#### REST API endpoints

* Some of the endpoints are rate limited per IP Address to 1000 requests per second.
If an IP Address exceeds the limit, the response would be an error with code `429` and message `Too Many Requests`.

* Some of the types have constant size in bytes, as specified below:

    |Type|Size in bytes|
    |---|---|
    |Account ID|32|
    |Public Key|32|
    |Signature|64|
    |Transaction / Contract ID|32|
    |Merkle ID|16|
    |Round ID|32|

**Ledger**
----
   Get ledger current status.
   
   This endpoint is rate limited.

* **URL**

    `/ledger`

* **Method:**

    `GET`
  
*  **URL Params**

    None

* **Data Params**

    None

* **Success Response:**

    * **Code:** 200 <br />
    **Content:**
    ```json
    {
      "public_key": "8419234abed4b6ad7ebf52f4c3f91a2ebcb93df2bd9fbd7f6a1946427d8806d8",
      "address": "127.0.0.1:49891",
      "num_accounts": 3,
      "round": {
        "merkle_root": "1099b75eef1423a90583a1daec9fdfb9",
        "start_id": "05937426b327cb0c4ea37f636d2fb59a8e2557932f9231f0b978cbb2f6824618",
        "end_id": "2f4b215400d14c1414f5579e994798c0a7ca677c4dc3a6a5f8159aaae0335c6f",
        "applied": 17160,
        "depth": 429,
        "difficulty": 10
      },
      "peers": [
        {
          "address": "127.0.0.1:49894",
          "public_key": "c88e21ab067593c4cfed850f2044a075132878e14dbf4972853f80294b1accc4"
        },
        {
          "address": "127.0.0.1:49900",
          "public_key": "451634618a41a8904eae8da506227a049b1ff57def42ba27e1a2b1423a5badbf"
        }
      ]
    }
    ```
 
* **Error Response:**

    * **Code:** 429 TOO MANY REQUEST <br />
    **Content:** `Too Many Requests`

**Account**
----
Get Account Information

* **URL**

    `/accounts/:id`

* **Method:**

    `GET`
  
*  **URL Params**

    `id=[string]` where `id` is the hex-encoded Account ID.

* **Data Params**

    None

* **Success Response:**

    * **Code:** 200 <br />
    **Content:**
    ```json
    {
      "public_key": "f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467",
      "balance": 9999999999999999901,
      "stake": 100,
      "reward": 0,
      "nonce": 152,
      "is_contract": false
    }
    ```
 
* **Error Response:**
    
    * **Reason:** Invalid Account ID size <br />
    **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "account ID must be 32 bytes long"
    }
    ```
    <br />
    
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "account ID must be presented as valid hex: [...]"
    }
    ```
 
 **Send Transaction**
 ----
   Send Transaction
 
* **URL**
 
    `/tx/send`
 
* **Method:**
 
    `POST`
   
*  **URL Params**
 
    None
 
* **Data Params**
 
    ```json
    {
      "sender": "[hex-encoded sender ID, must be 32 bytes long]",
      "tag": "[possible values: 0 = nop, 1 = transfer, 2 = contract, 3 = stake, 4 = batch",
      "payload": "[hex-encoded payload, empty for nop]",
      "signature": "[hex-encoded edwards25519 signature, which consists of private key, nonce, tag, and payload]"
    }
    ```
 
* **Success Response:**
 
    * **Code:** 200 <br />
    **Content:**
    ```json
    {
      "tx_id": "facd9c4bddc8d1080bac6d08a35cbd98ff9ef3924624d1307eced3b40d3549a0",
      "parent_ids": [
        "4a987f2371c27a23516ce8c93367826bd5a46bf815d0c19b765a5b10292e575b"
      ],
      "is_critical": false
    }
    ```
  
* **Error Response:**
 
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "error adding your transaction to graph: [...]"
    }
    ```

**Transaction List**
 ----
 
   Get Transaction List
 
   This endpoint is rate limited.
 
* **URL**
 
    `/tx`
 
* **Method:**
 
    `GET`
   
*  **URL Params**
 
    * **Optional Query:**
        
        `sender=[string]` where `sender` is the hex-encoded Sender ID. Used to filter by Sender ID.
                
        `offset=[integer]` where `offset` is page offset. Default 0.
        
        `limit=[integer]` where `limit` is page limit. If 0, it'll return all transactions.
        
* **Data Params**
 
    None
 
* **Success Response:**
 
    * **Code:** 200 <br />
    **Content:**
    ```json
    [
      {
        "id": "e6d9aab2d62522073daa0a30c629516fb7beb11d6a327f53eb6f6768dc6dbe09",
        "sender": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
        "status": "applied",
        "nonce": 0,
        "depth": 1172,
        "tag": 0,
        "payload": "",
        "sender_signature": "cc86583a84ca27b5860e3c3f6e75994b91ae4bd2ef8e4751b38141a098518ea84e9a8c0a374730ee5a75913117169ada1162fb9e569183b985bedbcf3d7c260b",
        "parents": [
          "eb148b6831275941c2b840dd21549a203335263cabce6a1de86a243704388ed4"
        ]
      }
    ]
    ```
  
* **Error Response:**
 
    * **Code:** 429 TOO MANY REQUEST <br />
    **Content:** `Too Many Requests`

**Transaction Detail**
 ----
   Get Transaction Details by ID
 
* **URL**
 
    `/tx/:id`
 
* **Method:**
 
    `GET`
   
*  **URL Params**
 
    `id=[string]` where `id` is the hex-encoded Transaction ID.
 
* **Data Params**
 
    None
 
* **Success Response:**
 
    * **Code:** 200 <br />
    **Content:**
    ```json
    {
      "id": "e6d9aab2d62522073daa0a30c629516fb7beb11d6a327f53eb6f6768dc6dbe09",
      "sender": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
      "status": "applied",
      "nonce": 0,
      "depth": 1172,
      "tag": 0,
      "payload": "",
      "sender_signature": "cc86583a84ca27b5860e3c3f6e75994b91ae4bd2ef8e4751b38141a098518ea84e9a8c0a374730ee5a75913117169ada1162fb9e569183b985bedbcf3d7c260b",
      "parents": [
        "eb148b6831275941c2b840dd21549a203335263cabce6a1de86a243704388ed4"
      ]
    }
    ```
  
* **Error Response:**
    
    * **Code:** 400 BAD REQUEST <br />
    **Desc:**  The Transaction ID is not a valid size. Its size must be 32 bytes<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "transaction ID must be 32 bytes long"
    }
    ```
    
    <br />
  
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** Transaction ID is not a valid hex<br />
    **Content:**
    ```json
    {
     "status": "Bad request.",
    "error": "transaction ID must be presented as valid hex: [...]"
    }
    ```
    
    <br />
  
    * **Code:** 404 NOT FOUND <br />
    **Desc:** Transaction ID does not exist<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "could not find transaction with ID [...]"
    }
    ```

**Contract Code**
 ----
   Get Contract Code By ID.
   
   This endpoint is rate limited.
 
* **URL**
 
    `/contract/:id`
 
* **Method:**
 
    `GET`
   
*  **URL Params**
 
    `id=[string]` where `id` is the hex-encoded Contract ID.
 
* **Data Params**
 
    None
 
* **Success Response:**
 
    * **Code:** 200 <br />
    **Content:**
    Th contract web assembly code.
  
* **Error Response:**
    
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** The Contract ID is not a valid size. Its size must be 32 bytes<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "contract ID must be 32 bytes long"
    }
    ```
    
    <br />
  
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** The contract ID is not a valid hex<br />
    **Content:**
    ```json
    {
     "status": "Bad request.",
    "error": "contract ID must be presented as valid hex: [...]"
    }
    ```
    
    <br />
  
    * **Code:** 404 NOT FOUND <br />
    **Desc:** The Contract ID does not exist<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "could not find contract with ID [...]"
    }
    ```
    
    <br />
  
    * **Code:** 429 TOO MANY REQUEST <br />
    **Desc:** The request is rate limited<br />
    **Content:** `Too Many Requests`
    
**Contract Page**
 ----
   Get Contract Page
   
   This endpoint is rate limited.
 
* **URL**
 
    `/contract/:id/page/:index`
 
* **Method:**
 
    `GET`
   
*  **URL Params**
 
    `id=[string]` where `id` is the hex-encoded Contract ID.
    
    `page=[integer]` where `id` is the page index.
 
* **Data Params**
 
    None
 
* **Success Response:**
 
    * **Code:** 200 <br />
    **Content:**
    The memory snapshot.
  
* **Error Response:**
    
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** The Contract ID is not a valid size. Its size must be 32 bytes<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "contract ID must be 32 bytes long"
    }
    ```
    
    <br />
  
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** The contract ID is not a valid hex<br />
    **Content:**
    ```json
    {
     "status": "Bad request.",
    "error": "contract ID must be presented as valid hex: [...]"
    }
    ```
    
    <br />
  
    * **Code:** 404 NOT FOUND <br />
    **Desc:** The contract does not have any page<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "could not find any pages for contract with ID [...]"
    }
    ```
    
    <br />
      
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** The page is either empty or does not exist <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "page [...] is either empty, or does not exist"
    }
    ```
    
    <br />
      
    * **Code:** 400 BAD REQUEST <br />
    **Desc:** The index does not exist<br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "contract with ID [...] only has [...] pages, but you requested page [...]"
    }
    ```
    
    <br />
  
    * **Code:** 429 TOO MANY REQUEST <br />
    **Desc:** The request is rate limited<br />
    **Content:** `Too Many Requests`
