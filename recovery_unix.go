package wavelet

import (
	"os"
	"syscall"
)

func (d *StallDetector) tryRestart() error {
	return syscall.Exec(os.Args[0], os.Args, os.Environ()) // nolint:gosec
}
