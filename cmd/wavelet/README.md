```bash
go run main.go -api.port 9000

# or use a config file
go run main.go -config config.toml
```

```bash
go run main.go -db.path testdb2 -port 3001 -privkey random -peers tcp://127.0.0.1:3000
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
