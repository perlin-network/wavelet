## Running benchmarks

Each `.go` file contains a runnable benchmark with a custom CSV output. You have to use `go run` instead of `go test` to run it, e.g.:

```sh
$ go run accounts.go | tee output.csv
```

