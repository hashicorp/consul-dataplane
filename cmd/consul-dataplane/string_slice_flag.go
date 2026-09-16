// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"flag"
	"fmt"
)

var _ flag.Value = (*FlagStringSliceValue)(nil)

// FlagStringSliceValue is a flag implementation that accumulates every value it
// is given, so the flag may be passed multiple times.
type FlagStringSliceValue []string

func (s *FlagStringSliceValue) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *FlagStringSliceValue) Set(value string) error {
	// Skip empty values rather than erroring, so that passing the flag with an
	// empty value remains the no-op it was when this was a single string flag.
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}
