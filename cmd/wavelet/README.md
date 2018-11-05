```bash
go run main.go -api.port 9000

# or use a config file
go run main.go -config config.toml --daemon=false
```

```bash
go run main.go --db.path testdb2 --port 3001 --private_key_file random --peers tcp://127.0.0.1:3000
```

```bash
# commands [if you just press enter, it'll send a nop transaction]

# Pays a random address should [address] not be specified.
# Pays 1 PERL by default unless [amount] is specified.
p [address] [amount]

# Gives details about your own personal wallet. If [account id] is specified, gives details of another wallet.
w [account id]

# Gives details about a transaction under id [tx id].
tx [tx id]

# Test-deploy a smart contract at a given path.
c [smart contract path here]

# Register yourself as a validator with a placed stake of [stake amount].
ps [stake amount]

# Withdraw [stake amount] from your stakes as a validator into PERLs.
ws [stake amount]

```
