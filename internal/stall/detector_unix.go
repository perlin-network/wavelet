// +build !windows

package stall

import (
	"os"
	"syscall"
)

func (d *Detector) TryRestart() error {
	return syscall.Exec(os.Args[0], os.Args, os.Environ()) // nolint:gosec
}
