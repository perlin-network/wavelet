#### REST API endpoints

**Ledger**
----
   Get ledger current status

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

    `/account/:id`

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

    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "account ID must be 32 bytes long"
    }
    ```
 
    OR

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
      "sender": "[hex-encoded sender ID]",
      "tag": "[0 = nop, 1 = transfer, 2 = contract, 3 = stake, 4 = batch",
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
 
* **URL**
 
    `/tx`
 
* **Method:**
 
    `GET`
   
*  **URL Params**
 
    * **Query:**
        
        `sender=[string]` where `sender` is the hex-encoded Sender ID. Used to filter by Sender ID.
        
        `creator=[string]` where `creator` is the hex-encoded Creator ID. Used to filter by Creator ID.
        
        `offset=[integer]` where `offset` is page offset.
        
        `limit=[integer]` where `limit` is page limit.
        
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
        "creator": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
        "status": "applied",
        "nonce": 0,
        "depth": 1172,
        "tag": 0,
        "payload": "",
        "sender_signature": "cc86583a84ca27b5860e3c3f6e75994b91ae4bd2ef8e4751b38141a098518ea84e9a8c0a374730ee5a75913117169ada1162fb9e569183b985bedbcf3d7c260b",
        "creator_signature": "fbd433221d4a9a6345dc184aad39b118266376a538c5e3cdba28a6ea5435bb6c776b31aa95eda803881df39cdda164294c61b9475a2e9dcdcfbc7f5efd1b3404",
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
   Get Transaction Detail
 
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
      "creator": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
      "status": "applied",
      "nonce": 0,
      "depth": 1172,
      "tag": 0,
      "payload": "",
      "sender_signature": "cc86583a84ca27b5860e3c3f6e75994b91ae4bd2ef8e4751b38141a098518ea84e9a8c0a374730ee5a75913117169ada1162fb9e569183b985bedbcf3d7c260b",
      "creator_signature": "fbd433221d4a9a6345dc184aad39b118266376a538c5e3cdba28a6ea5435bb6c776b31aa95eda803881df39cdda164294c61b9475a2e9dcdcfbc7f5efd1b3404",
      "parents": [
        "eb148b6831275941c2b840dd21549a203335263cabce6a1de86a243704388ed4"
      ]
    }
    ```
  
* **Error Response:**
    
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "transaction ID must be 32 bytes long"
    }
    ```
    
    OR
  
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
     "status": "Bad request.",
    "error": "transaction ID must be presented as valid hex: [...]"
    }
    ```
    
    OR
  
    * **Code:** 404 NOT FOUND <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "could not find transaction with ID [...]"
    }
    ```

**Contract Code**
 ----
   Get Contract Code By ID
 
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
    Hex-encoded of the contract web assembly code.
  
* **Error Response:**
    
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "contract ID must be 32 bytes long"
    }
    ```
    
    OR
  
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
     "status": "Bad request.",
    "error": "contract ID must be presented as valid hex: [...]"
    }
    ```
    
    OR
  
    * **Code:** 404 NOT FOUND <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "could not find contract with ID [...]"
    }
    ```
    OR
  
    * **Code:** 429 TOO MANY REQUEST <br />
    **Content:** `Too Many Requests`
    
**Contract Page**
 ----
   Get Contract Page
 
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
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "contract ID must be 32 bytes long"
    }
    ```
    
    OR
  
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
     "status": "Bad request.",
    "error": "contract ID must be presented as valid hex: [...]"
    }
    ```
    
    OR
  
    * **Code:** 404 NOT FOUND <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "could not find any pages for contract with ID [...]"
    }
    ```
    
    OR
      
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "page [...] is either empty, or does not exist"
    }
    ```
    
    OR
      
    * **Code:** 400 BAD REQUEST <br />
    **Content:**
    ```json
    {
      "status": "Bad request.",
      "error": "contract with ID [...] only has [...] pages, but you requested page [...]"
    }
    ```
    
    OR
  
    * **Code:** 429 TOO MANY REQUEST <br />
    **Content:** `Too Many Requests`