package flamenco

// MaxInt returns the maximum of a and b.
func MaxInt(a, b int) int {
	if a < b {
		return b
	}
	return a
}
