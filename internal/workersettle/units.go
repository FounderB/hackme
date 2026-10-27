package workersettle

import "hackme/internal/chain"

// UnitsPerHMC matches on-chain / Dig-Hunt escrow (1 HMC = 1e8 units).
const UnitsPerHMC = chain.UnitsPerHMC

// HMCToUnits converts float HMC to integer units (round half-up).
func HMCToUnits(v float64) uint64 { return chain.HMCToUnits(v) }

// UnitsToHMC converts integer units to float HMC for API/UI mirrors.
func UnitsToHMC(v uint64) float64 { return chain.UnitsToHMC(v) }
