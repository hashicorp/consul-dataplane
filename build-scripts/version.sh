#!/usr/bin/env bash
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0


version_file=$1
version=$(awk -F- '{ print $1 }' < "${version_file}")
prerelease=$(awk -F- '{ print $2 }' < "${version_file}")

if [ -n "$prerelease" ]; then
    echo "${version}-${prerelease}"
else
    echo "${version}"
fi
