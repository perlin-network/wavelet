package node

import (
	"github.com/perlin-network/wavelet/security"
	"math/big"
)

// generateRewardProof returns the coefficients of a N-degree polynomial constructed through N provided roots. These N roots
// are the hashes of a set of N public keys.
func generateRewardProof(ids []string, p *big.Int) []*big.Int {
	roots := make([]*big.Int, len(ids))
	for i, id := range ids {
		roots[i] = new(big.Int).SetBytes(security.Hash([]byte(id)))
	}

	return polynomialFromRoots(roots, p)
}

// polynomialFromRoots constructs a N-degree polynomial given N roots specified as arbitrary-precision integers.
func polynomialFromRoots(roots []*big.Int, p *big.Int) []*big.Int {
	coeffs := make([]*big.Int, len(roots)+1)
	// -coeffs[0] = roots[0] % p
	coeffs[0] = new(big.Int).Neg(new(big.Int).Mod(roots[0], p))
	coeffs[1] = big.NewInt(1)

	for k := 1; k < len(roots); k++ {
		// root = roots[k] % p
		root := new(big.Int).Mod(roots[k], p)
		coeffs[k+1] = big.NewInt(1)

		for i := k - 1; i >= 0; i-- {
			// coeffs[i + 1] = (coeffs[i] - coeffs[i + 1] * root) % p
			coeffs[i+1] = new(big.Int).Mod(new(big.Int).Sub(coeffs[i], new(big.Int).Mul(coeffs[i+1], root)), p)
		}
		// coeffs[0] = -(root * coeffs[0]) % p
		coeffs[0] = new(big.Int).Mod(new(big.Int).Neg(new(big.Int).Mul(root, coeffs[0])), p)
	}

	for i := 0; i < len(coeffs); i++ {
		if coeffs[i].Cmp(big.NewInt(0)) < 0 {
			coeffs[i].Add(coeffs[i], p)
		}
	}

	return coeffs
}

func peerDeservesReward(coeffs []*big.Int, id string, p *big.Int) bool {
	x := new(big.Int).SetBytes(security.Hash([]byte(id)))

	zero := big.NewInt(0)

	// assert 0 <= x < p
	if !(x.Cmp(zero) >= 0 && x.Cmp(p) < 0) {
		return false
	}

	result := coeffs[0]

	for i, coeff := range coeffs[1:] {
		// result = (result + coeff * (x ** (i + 1))) % p
		result = new(big.Int).Mod(new(big.Int).Add(result, new(big.Int).Mul(coeff, new(big.Int).Exp(x, big.NewInt(int64(i+1)), nil))), p)
	}

	return result.Cmp(zero) == 0
}
