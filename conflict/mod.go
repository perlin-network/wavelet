package conflict

import (
	"github.com/perlin-network/wavelet/common"
)

type Resolver interface {
	Reset()
	Tick(id common.TransactionID, votes []float64)

	Prefer(id common.TransactionID)
	Preferred() common.TransactionID

	Decided() bool
}
