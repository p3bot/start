package cue

import (
	"errors"
	"math"

	"cuelang.org/go/cue"
)

// PositiveInt reads field as a whole number of 1 or more. An absent field is
// (0, nil). A present value outside that range is an error, so a caller that
// writes the value back cannot drop it.
func PositiveInt(parent cue.Value, field string) (int, error) {
	v := parent.LookupPath(cue.ParsePath(field))
	if !v.Exists() {
		return 0, nil
	}
	n, err := v.Int64()
	if err != nil || n < 1 || n > math.MaxInt {
		return 0, errors.New("must be a whole number of 1 or more")
	}
	return int(n), nil
}
