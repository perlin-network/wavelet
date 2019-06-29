# Payloads

Payloads are commonly attached to transactions in order to represent complex
statements. For example, should a user wish to send PERLs to an arbitrary account via the Transfer
tag, they will attach a payload to that transaction specifying the recipient, the amount of PERLs
to send, and possibly other contract-related details (function names, gas limits, and function payloads).

## Payloads and Wctl

In order to send a payload along with a transaction, a user must provide a JSON-formatted payload file
via the `--payload` flag. The names of each of the aforementioned payload fields are as follows:

### Transfer

| Payload Field        | JSON Field |
| -------------------- | ---------- |
| Recipient Account ID | recipient  |
| Num PERLs Sent       | amount     |
| Gas Limit            | gas_limit  |
| Function Name        | fn_name    |
| Function Payload     | fn_payload |

### Stake

| Payload Field | JSON Field                      |
|---------------|---------------------------------|
| Operation     | operation (string, e.g. "0x00") |
| Amount        | amount                          |

### Contract

### Batch
