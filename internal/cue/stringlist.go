package cue

import (
	"fmt"

	"cuelang.org/go/cue"
)

// StringList reads field as a list of strings.
// An absent field is (nil, nil). A present value that is not a list of
// strings is an error, so a caller that writes the value back cannot drop it.
// A present empty list is a non-nil empty slice.
func StringList(parent cue.Value, field string) ([]string, error) {
	v := parent.LookupPath(cue.ParsePath(field))
	if !v.Exists() {
		return nil, nil
	}
	iter, err := v.List()
	if err != nil {
		return nil, fmt.Errorf("not a list of strings: %w", err)
	}
	items := []string{}
	for iter.Next() {
		s, err := iter.Value().String()
		if err != nil {
			return nil, fmt.Errorf("not a list of strings: %w", err)
		}
		items = append(items, s)
	}
	return items, nil
}
