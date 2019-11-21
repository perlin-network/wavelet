// +build !windows

package main

import (
	"os"
	"syscall"
)

func switchToUpdatedBinary(newBinary string, atStartup bool) error { // nolint:unused
	origArg0 := os.Args[0]

	os.Args[0] = newBinary

	err := syscall.Exec(os.Args[0], os.Args, os.Environ()) // nolint:gosec

	os.Args[0] = origArg0

	return err
}
