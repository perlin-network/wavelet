package wavelet

import "github.com/pkg/errors"

var (
	ErrStopped       = errors.New("worker stopped")
	ErrNonePreferred = errors.New("no critical transactions available in round yet")
	ErrTimeout       = errors.New("timed out")
)
