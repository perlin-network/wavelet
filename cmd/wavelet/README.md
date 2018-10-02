```bash
go run main.go -api 9000
```

```bash
go run main.go -db testdb2 -p 3001 -sk random -n tcp://127.0.0.1:3000
```

```bash
# commands [if you just press enter, it'll send a nop transaction]

# Pays a random address should [address] not be specified.
# Pays 1 PERL by default unless [amount] is specified.
p [address] [amount]

# Gives details about your own personal wallet.
w

# Gives details about an account given [account id].
a [account id]

# Test-deploy a smart contract at a given path.
c [smart contract path here]



```