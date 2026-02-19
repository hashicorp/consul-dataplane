#!/usr/bin/env bash
# Copyright IBM Corp. 2022, 2025
# SPDX-License-Identifier: MPL-2.0

function sed_i {
	if test "$(uname)" == "Darwin"; then
		sed -i '' "$@"
		return $?
	else
		sed -i "$@"
		return $?
	fi
}

if test "$(uname)" == "Darwin"; then
	SED_EXT="-E"
else
	SED_EXT="-r"
fi

VFILE=$1
VERSION=$2
PRERELEASE=$3

echo "==> Preparing consul-dataplane for release by updating ${VFILE} with version info: ${VERSION}"

sed_i ${SED_EXT} -e "s/(Version[[:space:]]*=[[:space:]]*)\"[^\"]*\"/\1\"${VERSION}\"/g" -e "s/(VersionPrerelease[[:space:]]*=[[:space:]]*)\"[^\"]*\"/\1\"${PRERELEASE}\"/g" "${VFILE}"
