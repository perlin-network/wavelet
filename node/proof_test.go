package node

import (
	"github.com/perlin-network/wavelet/params"
	"testing"
)

func TestGenerateRewardProof(t *testing.T) {
	ids := []string{"alice", "bob", "charlie"}

	coeffs := generateRewardProof(ids, params.ConsensusRewardProofP)
	t.Log(coeffs)
}

func TestProof(t *testing.T) {
	ids := []string{"alice", "bob", "charlie"}

	coeffs := generateRewardProof(ids, params.ConsensusRewardProofP)
	for _, id := range ids {
		t.Log(peerDeservesReward(coeffs, id, params.ConsensusRewardProofP))
	}

	t.Log(peerDeservesReward(coeffs, "should_fail", params.ConsensusRewardProofP))
}
