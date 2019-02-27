package wavelet

import "github.com/pkg/errors"

var (
	VoteAccepted = errors.New("vote accepted")
	VoteRejected = errors.New("vote rejected")
)
