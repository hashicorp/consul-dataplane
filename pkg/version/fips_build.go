// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build fips

package version

// This reports runtime information for FIPS-enabled builds.
import (
	"crypto/fips140"
)

// IsFIPS returns whether consul-dataplane is operating in FIPS mode.
func IsFIPS() bool {
	return true
}

func GetFIPSInfo() string {
	moduleVersion := fips140.Version()
	if moduleVersion == "" {
		return "FIPS 140-3 Enabled"
	}

	return "FIPS 140-3 Enabled, crypto module " + moduleVersion
}
