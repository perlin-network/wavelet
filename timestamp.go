package wavelet

import "sort"

func computeMedianTimestamp(timestamps []uint64) uint64 {
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	var median uint64

	if len(timestamps)%2 == 0 {
		median = (timestamps[len(timestamps)/2-1] / 2) + (timestamps[len(timestamps)/2] / 2)
	} else {
		median = timestamps[len(timestamps)/2]
	}

	return median
}

func computeMeanTimestamp(timestamps []uint64) (mean uint64) {
	for _, timestamp := range timestamps {
		mean += timestamp / uint64(len(timestamps))
	}

	return
}
