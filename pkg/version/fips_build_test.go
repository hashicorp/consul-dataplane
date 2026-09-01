// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build fips

package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsFIPS_FIPSBuild(t *testing.T) {
	require.True(t, IsFIPS())
}

func TestGetFIPSInfo_FIPSBuildReports1403(t *testing.T) {
	info := GetFIPSInfo()
	require.Contains(t, info, "FIPS 140-3 Enabled")
}

func TestGetHumanVersion_FIPSBuildHas1403Suffix(t *testing.T) {
	withVersion(t, "1.2.3", "dev")

	got := GetHumanVersion()
	require.Contains(t, got, "+fips1403")
	require.NotContains(t, got, "+fips1402")
}
