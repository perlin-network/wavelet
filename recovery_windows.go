package wavelet

import "github.com/pkg/errors"

func (d *StallDetector) tryRestart() error {
	return errors.New("Restart is not supported on Windows.")
}
