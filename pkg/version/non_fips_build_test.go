// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !fips

package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsFIPS_NonFIPSBuild(t *testing.T) {
	require.False(t, IsFIPS())
}

func TestGetFIPSInfo_NonFIPSBuild(t *testing.T) {
	require.Empty(t, GetFIPSInfo())
}

func TestGetHumanVersion_NonFIPSBuildHasNoFIPSSuffix(t *testing.T) {
	withVersion(t, "1.2.3", "dev")

	got := GetHumanVersion()
	require.NotContains(t, got, "+fips1403")
	require.NotContains(t, got, "+fips1402")
}
