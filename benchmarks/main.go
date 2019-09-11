package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("usage: go run . <benchmark>")
		os.Exit(1)
	}

	benchmark := args[0]
	switch benchmark {
	case "accounts":
		runAccountsBenchmark()

	case "tree":
		runTreeBenchmark()
	}
}
