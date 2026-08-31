// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"fmt"
	"strings"
)

var (
	// The git commit that was compiled. These will be filled in by the
	// compiler.
	GitCommit string

	// The main version number that is being run at the moment.
	//
	// Version must conform to the format expected by github.com/hashicorp/go-version
	// for tests to work.
	Version = "2.1.0"

	// A pre-release marker for the version. If this is "" (empty string)
	// then it means that it is a final release. Otherwise, this is a pre-release
	// such as "dev" (in development), "beta", "rc1", etc.
	VersionPrerelease = "dev"
)

// GetHumanVersion composes the parts of the version in a way that's suitable
// for displaying to humans.
func GetHumanVersion() string {
	version := Version
	release := VersionPrerelease

	if release != "" {
		if !strings.HasSuffix(version, "-"+release) {
			// if we tagged a prerelease version then the release is in the version already
			version += fmt.Sprintf("-%s", release)
		}
	}

	version = withFIPSSuffix(version, IsFIPS())

	// Strip off any single quotes added by the git information.
	return strings.ReplaceAll(version, "'", "")
}

// withFIPSSuffix appends the FIPS 140-3 build marker to version when fips is
// true. Split out from GetHumanVersion so this branch can be exercised by a
// plain unit test, without requiring a fips-tagged build (see IsFIPS, which
// is only ever true when compiled with -tags fips).
func withFIPSSuffix(version string, fips bool) string {
	if fips {
		return fmt.Sprintf("%s+fips1403", version)
	}

	return version
}
