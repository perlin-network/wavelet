package wavelet

import "github.com/perlin-network/wavelet/sys"

func computeNextDifficulty(critical *Transaction) uint64 {
	timestamps := make([]uint64, len(critical.DifficultyTimestamps))
	copy(timestamps, critical.DifficultyTimestamps)

	timestamps = append(timestamps, critical.Timestamp)

	// Prune away critical timestamp history if needed.
	if size := computeCriticalTimestampWindowSize(critical.ViewID); len(timestamps) > size+1 {
		timestamps = timestamps[len(timestamps)-size+1:]
	}

	var deltas []uint64

	// Compute first-order differences across timestamps.
	for i := 1; i < len(timestamps); i++ {
		deltas = append(deltas, timestamps[i]-timestamps[i-1])
	}

	mean := computeMeanTimestamp(deltas)

	expectedTimeFactor := float64(sys.ExpectedConsensusTimeMilliseconds) / float64(mean)
	if expectedTimeFactor > 10.0 {
		expectedTimeFactor = 10.0
	}

	difficulty := sys.MinDifficulty + int(float64(sys.MaxDifficulty-sys.MinDifficulty)/10.0*expectedTimeFactor)

	return uint64(difficulty)
}
