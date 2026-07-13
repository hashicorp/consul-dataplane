// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// withVersion temporarily overrides the package-level version globals for the
// duration of a test and restores them afterwards.
func withVersion(t *testing.T, v, prerelease string) {
	t.Helper()

	origVersion := Version
	origPrerelease := VersionPrerelease
	t.Cleanup(func() {
		Version = origVersion
		VersionPrerelease = origPrerelease
	})

	Version = v
	VersionPrerelease = prerelease
}

func TestGetHumanVersion_Prerelease(t *testing.T) {
	withVersion(t, "1.2.3", "dev")

	got := GetHumanVersion()
	require.Contains(t, got, "1.2.3-dev")
}

func TestGetHumanVersion_FinalRelease(t *testing.T) {
	withVersion(t, "1.2.3", "")

	got := GetHumanVersion()
	require.Contains(t, got, "1.2.3")
	require.NotContains(t, got, "-dev")
}

func TestGetHumanVersion_StripsSingleQuotes(t *testing.T) {
	withVersion(t, "1.2.3'", "")

	got := GetHumanVersion()
	require.NotContains(t, got, "'")
}
