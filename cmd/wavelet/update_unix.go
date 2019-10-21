package main

import (
	"os"
	"syscall"
)

func switchToUpdatedBinary(newBinary string, atStartup bool) error {
	origArg0 := os.Args[0]

	os.Args[0] = newBinary

	err := syscall.Exec(os.Args[0], os.Args, os.Environ())

	os.Args[0] = origArg0

	return err
}
