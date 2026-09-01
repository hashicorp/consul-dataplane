// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !fips

package version

// IsFIPS returns whether consul-dataplane is operating in FIPS mode.
func IsFIPS() bool {
	return false
}

func GetFIPSInfo() string {
	return ""
}
