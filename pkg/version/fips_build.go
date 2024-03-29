// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build fips

package version

// This validates during compilation that we are being built with a FIPS enabled go toolchain
import (
	_ "crypto/tls/fipsonly"
	"runtime"
	"strings"
)

// IsFIPS returns true if consul-dataplane is operating in FIPS-140-2 mode.
func IsFIPS() bool {
	return true
}

func GetFIPSInfo() string {
	str := "Enabled"
	// Try to get the crypto module name
	gover := strings.Split(runtime.Version(), "X:")
	if len(gover) >= 2 {
		gover_last := gover[len(gover)-1]
		// Able to find crypto module name; add that to status string.
		str = "FIPS 140-2 Enabled, crypto module " + gover_last
	}
	return str
}
