#### Websocket endpoints

* To prevent flooding, the endpoints, /accounts, /contract, and /tx are debounced. The debounce can also has a buffer, if specified.<br />
The way the debounce works is, if there's no new event within certain duration or the buffer is full, then the buffer will be released. <br />
To prevent duplicated events, debounce can also dedupe the events based on certain properties. <br />
Below are conditions for each of the debounced endpoints :
    * `/poll/accounts` is debounced with a duration of 500 milliseconds and deduped using `account_id` and `event`
    * `/poll/contract` is debounced with a duration of 500 milliseconds and deduped using `contract_id`.
    * `/poll/tx` is debounced with a duration of 2200 milliseconds and buffer size of around 1.6 MB.


**Poll Accounts**
 ----
   Listen to account events 
 
* **URL**
 
    `/poll/accounts`

* **Query Param:**

    Optional parameters to filter the events by certain properties.
            
    `id=[string]` where `id` is the hex-encoded Account ID.
 
* **Message:**

    A JSON array of events.
 
    * **Event:** Balance Updated <br />
    ```json
    {
      "mod": "accounts",
      "event": "balance_updated",
      "account_id": "746843bbdaa98008d11f705a01dc1c8dccbdf80d4f2ed657238eb2f05325a523",
      "balance": 6,
      "time": "2019-06-28T15:29:37+08:00"
      }
    ```
    
    * **Event:** Contract Num Pages Updated <br />
    ```json
    {
      "mod": "accounts",
      "event": "num_pages_updated",
      "account_id": "2702d3247117f138ea3d7c3a386fd56c75a0d23048178f4b8dba94651c4ff9b0",
      "num_pages": 18,
      "time": "2019-06-28T19:24:53+08:00"
    }
    ```
    
    * **Event:** Stake Updated <br />
    ```json
    {
      "mod": "accounts",
      "event": "stake_updated",
      "account_id": "696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a",
      "stake": 1000,
      "time": "2019-06-28T19:28:09+08:00"
    }
    ```
    
    * **Event:** Reward Updated <br />
    ```json
    {
      "mod": "accounts",
      "event": "reward_updated",
      "account_id": "696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a",
      "reward": 18,
      "time": "2019-06-28T19:29:36+08:00"
    }
    ```
  
**Poll Network**
 ----
   Listen to network events 
 
* **URL**
 
    `/poll/network`
   
*  **Query Params**
 
    None
 
* **Message:**

    A JSON object of the event.
 
    * **Event:** Peer Joined<br />
    ```json
    {
      "level": "info",
      "mod": "network",
      "event": "joined",
      "public_key": "f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467",
      "address": "127.0.0.1:3002",
      "time": "2019-06-28T19:18:15+08:00",
      "message": "Peer has joined."
    }
    ```
    
    * **Event:** Peer Left<br />
    ```json
    {
      "level": "info",
      "mod": "network",
      "event": "left",
      "public_key": "f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467",
      "address": "127.0.0.1:3002",
      "time": "2019-06-28T19:18:29+08:00",
      "message": "Peer has left."
    }
    ```

**Poll Consensus**
 ----
   Listen to Consensus events 
 
* **URL**
 
    `/poll/consensus`
   
* **Query Params**
 
    None

* **Message:**

    A JSON object of the event.
 
    * **Event:** Round End<br />
    ```json
    {
      "level": "info",
      "mod": "consensus",
      "event": "round_end",
      "num_applied_tx": 352,
      "num_rejected_tx": 0,
      "num_ignored_tx": 0,
      "old_round": 8,
      "new_round": 9,
      "old_difficulty": 8,
      "new_difficulty": 8,
      "new_root": "029ae28f06608050ccf53a7257345070909a4477f694940d56455862005c101c",
      "old_root": "c7fed1638209a0862b0e434745b40d2639e43225b3d9bcfa7cfdeb3028e0b72b",
      "new_merkle_root": "9d1c441f730006e2685238219690b66c",
      "old_merkle_root": "6dee79e9ea2147271a831f9d3d12f8ff",
      "round_depth": 352,
      "time": "2019-06-28T19:39:06+08:00",
      "message": "Finalized consensus round, and initialized a new round."
    }
    ```
    
    * **Event:** Prune<br />
    ```json
    {
      "level": "debug",
      "mod": "consensus",
      "event": "prune",
      "num_tx": 1,
      "current_round_id": "029ae28f06608050ccf53a7257345070909a4477f694940d56455862005c101c",
      "pruned_round_id": "c7fed1638209a0862b0e434745b40d2639e43225b3d9bcfa7cfdeb3028e0b72b",
      "time": "2019-06-28T20:38:13+08:00",
      "message": "Pruned away round and transactions."
    }
    ```

**Poll Contract**
 ----
   Listen to contract events 
 
* **URL**
 
    `/poll/contract`
 
* **Query Params:**

    Optional parameters to filter the events by certain properties.
            
    `id=[string]` where `id` is the hex-encoded Contract ID.
 
* **Message:**

    A JSON array of events.
 
    * **Event:** Gas for Invoke Contract<br />
    ```json
    {
      "level": "info",
      "mod": "contract",
      "event": "gas",
      "sender_id": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
      "contract_id": "2702d3247117f138ea3d7c3a386fd56c75a0d23048178f4b8dba94651c4ff9b0",
      "gas": 288512,
      "gas_limit": 9999999999999902985,
      "time": "2019-06-28T19:25:47+08:00",
      "message": "Deducted PERLs for invoking smart contract function."
    }
    ```
    
    * **Event:** Gas for Spawn Contract <br />
    ```json
    {
      "level": "info",
      "mod": "contract",
      "event": "gas",
      "contract_id": "66e7872ba4e4b6e0c7c874f0591b0439a957546df9b71936eee44347633055fa",
      "gas": 3484936,
      "gas_limit": 100000000,
      "time": "2019-06-28T20:34:06+08:00",
      "message": "Deducted PERLs for spawning a smart contract."
    }
    ```
    
    * **Event:** Exceed Gas Limit for Spawn Contract <br />
    ```json
    {
      "level": "info",
      "mod": "contract",
      "event": "gas",
      "sender_id": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
      "contract_id": "2702d3247117f138ea3d7c3a386fd56c75a0d23048178f4b8dba94651c4ff9b0",
      "gas": 1,
      "gas_limit": 1,
      "time": "2019-06-28T20:38:13+08:00",
      "message": "Exceeded gas limit while invoking smart contract function."
    }
    ```
    
    * **Event:** Contract Log <br />
    ```json
    {
      "level": "debug",
      "mod": "contract",
      "event": "log",
      "contract_id": "2702d3247117f138ea3d7c3a386fd56c75a0d23048178f4b8dba94651c4ff9b0",
      "time": "2019-06-28T20:38:13+08:00",
      "message": "[...]"
    }
    ```
    
**Poll Transaction** <br />
 ----
   Listen to transaction events 
 
* **URL**
 
    `/poll/tx`
  
* **Query Params:**

    Optional parameters to filter the events by certain properties.  
     
    `id=[string]` where `id` is the hex-encoded Transaction ID.

    `sender=[string]` where `sender` is the hex-encoded Sender ID.
                       
    `tag=[integer]` where `tag` is the tag.
 
* **Message:**

    A JSON array of events.
 
    * **Event:** Applied<br />
    ```json
    {
      "mod": "tx",
      "event": "applied",
      "tx_id": "9ba1e35eda41e67486ab12d0a6353aefb0dc8b8156aaecae357cf06cd49659b6",
      "sender_id": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
      "depth": 9747,
      "tag": 0,
      "time": "2019-06-28T20:48:17+08:00"
    }
    ```
    
    * **Event:** Gossip Error Batch<br />
    ```json
    {
      "level": "error",
      "mod": "tx",
      "event": "gossip",
      "error": "EOF",
      "time": "2019-06-28T20:49:52+08:00",
      "message": "Failed to send batch"
    }
    ```
    
    * **Event:** Gossip Error Unmarshal <br />
    ```json
    {
      "level": "error",
      "mod": "tx",
      "event": "gossip",
      "error": "EOF",
      "time": "2019-06-28T20:49:52+08:00",
      "message": "Failed to unmarshal transaction"
    }
    ```

    * **Event:** Failed<br />
    ```json
    {
      "mod": "tx",
      "event": "failed",
      "tx_id": "9ba1e35eda41e67486ab12d0a6353aefb0dc8b8156aaecae357cf06cd49659b6",
      "sender_id": "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405",
      "depth": 9747,
      "tag": 0,
      "error": "[...]",
      "time": "2019-06-28T20:48:17+08:00"
    }
    ```
    
**Poll Metrics**
 ----
   Listen to metrics events. The event will be sent every 1 second.
 
* **URL**
 
    `/poll/metrics`
 
* **Query Params**
 
    None
 
* **Message:**

    A JSON object of the event.
 
    * **Event:** Metrics<br />
    ```json
    {
      "level": "info",
      "mod": "metrics",
      "round.queried": 210858,
      "tx.gossiped": 9946,
      "tx.received": 9946,
      "tx.accepted": 9945,
      "tx.downloaded": 0,
      "rps.queried": 34.313755465462016,
      "tps.gossiped": 1.6185518808848753,
      "tps.received": 1.6185518810250006,
      "tps.accepted": 1.6183891471878615,
      "tps.downloaded": 0,
      "query.latency.max.ms": 2,
      "query.latency.min.ms": 0,
      "query.latency.mean.ms": 0.07082125700389105,
      "time": "2019-06-28T20:58:30+08:00",
      "message": "Updated metrics."
    }
    ```
