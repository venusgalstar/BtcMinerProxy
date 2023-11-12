package lib

import (
	"golang.org/x/exp/constraints"
)

type Number interface {
	constraints.Integer | constraints.Float
}

func AlmostEqual[T Number](a, b T, tolerance float64) bool {
	return RelativeError(a, b) < tolerance
}

// RelativeError returns relative error between two values
func RelativeError[T Number](target, actual T) float64 {
	return Abs(float64(actual)-float64(target)) / float64(Abs(target))
}

func Abs[T Number](a T) T {
	if a < 0 {
		return -a
	}
	return a
}
