package stall

import "github.com/pkg/errors"

func (d *Detector) TryRestart() error {
	return errors.New("Restart is not supported on Windows.")
}
