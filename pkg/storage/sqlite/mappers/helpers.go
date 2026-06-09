package mappers

import "math"

// safeUint64ToInt64 converts uint64 to int64 safely, capping at max int64.
func safeUint64ToInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// safeInt64ToUint64 converts int64 to uint64 safely, treating negative as 0.
func safeInt64ToUint64(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}
