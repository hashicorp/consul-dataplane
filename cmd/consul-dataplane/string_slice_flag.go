// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
)

var (
	_ flag.Value       = (*FlagStringSliceValue)(nil)
	_ json.Marshaler   = (*FlagStringSliceValue)(nil)
	_ json.Unmarshaler = (*FlagStringSliceValue)(nil)
)

// FlagStringSliceValue is a flag implementation that accumulates every value it
// is given, so the flag may be passed multiple times.
//
// It records whether it was supplied at all, separately from the values it
// collected. That distinction matters for precedence: supplying the flag with
// an empty value is an explicit request to use no values, and must override a
// lower-precedence configuration, whereas not supplying the flag at all must
// leave that configuration alone.
type FlagStringSliceValue struct {
	values []string
	set    bool
}

// NewFlagStringSliceValue returns a value holding the given URLs, marked as
// explicitly supplied. It is intended for tests.
func NewFlagStringSliceValue(values ...string) FlagStringSliceValue {
	return FlagStringSliceValue{values: values, set: true}
}

func (s *FlagStringSliceValue) String() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%v", s.values)
}

func (s *FlagStringSliceValue) Set(value string) error {
	// Record the occurrence even when the value is empty. An empty value clears
	// any previously accumulated values (e.g. from environment variables) so
	// that -flag= is an explicit request to use no values and overrides a
	// lower-precedence configuration. Non-empty values are appended, so
	// subsequent -flag=x after -flag= accumulate as expected.
	s.set = true
	if value == "" {
		s.values = nil
		return nil
	}
	s.values = append(s.values, value)
	return nil
}

// Values returns the collected values.
func (s FlagStringSliceValue) Values() []string {
	return s.values
}

// IsSet reports whether the flag, environment variable, or config file field
// was supplied, regardless of whether it yielded any values.
func (s FlagStringSliceValue) IsSet() bool {
	return s.set
}

func (s FlagStringSliceValue) MarshalJSON() ([]byte, error) {
	if !s.set {
		return []byte("null"), nil
	}
	if len(s.values) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(s.values)
}

func (s *FlagStringSliceValue) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}

	// A present field counts as supplied, so an explicit empty array in a
	// config file overrides a deprecated single URL in that same file.
	s.set = true
	for _, v := range values {
		if v == "" {
			continue
		}
		s.values = append(s.values, v)
	}
	return nil
}
