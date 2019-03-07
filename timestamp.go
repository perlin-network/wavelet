package wavelet

import (
	"github.com/perlin-network/wavelet/sys"
	"sort"
)

func computeMedianTimestamp(timestamps []uint64) (median uint64) {
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	if len(timestamps)%2 == 0 {
		median = (timestamps[len(timestamps)/2-1] / 2) + (timestamps[len(timestamps)/2] / 2)
	} else {
		median = timestamps[len(timestamps)/2]
	}

	return
}

func computeMeanTimestamp(timestamps []uint64) (mean uint64) {
	for _, timestamp := range timestamps {
		mean += timestamp / uint64(len(timestamps))
	}

	return
}

func computeCriticalTimestampWindowSize(viewID uint64) int {
	size := sys.CriticalTimestampAverageWindowSize
	if viewID+1 < uint64(size) {
		size = int(viewID) + 1
	}

	return size
}

func assertValidCriticalTimestamps(tx *Transaction) bool {
	if len(tx.DifficultyTimestamps) != computeCriticalTimestampWindowSize(tx.ViewID) {
		return false
	}

	// Check that difficulty timestamps are in ascending order.
	for i := 1; i < len(tx.DifficultyTimestamps); i++ {
		if tx.DifficultyTimestamps[i] < tx.DifficultyTimestamps[i-1] {
			return false
		}
	}

	return true
}
