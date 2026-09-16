// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlagStringSliceValue(t *testing.T) {
	t.Run("accumulates repeated flags in order", func(t *testing.T) {
		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.Var(&v, "url", "")

		err := fs.Parse([]string{
			"-url=http://127.0.0.1:8080/metrics",
			"-url=http://127.0.0.1:9090/admin/metrics",
			"-url", "http://127.0.0.1:7070/metrics",
		})
		require.NoError(t, err)
		require.Equal(t, FlagStringSliceValue{
			"http://127.0.0.1:8080/metrics",
			"http://127.0.0.1:9090/admin/metrics",
			"http://127.0.0.1:7070/metrics",
		}, v)
	})

	t.Run("a single flag yields a single value", func(t *testing.T) {
		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.Var(&v, "url", "")

		require.NoError(t, fs.Parse([]string{"-url=http://127.0.0.1:8080/metrics"}))
		require.Equal(t, FlagStringSliceValue{"http://127.0.0.1:8080/metrics"}, v)
	})

	t.Run("unset stays nil", func(t *testing.T) {
		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.Var(&v, "url", "")

		require.NoError(t, fs.Parse(nil))
		require.Nil(t, v)
	})

	t.Run("a value containing a comma is kept intact", func(t *testing.T) {
		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.Var(&v, "url", "")

		require.NoError(t, fs.Parse([]string{"-url=http://127.0.0.1:8080/metrics?labels=a,b"}))
		require.Equal(t, FlagStringSliceValue{"http://127.0.0.1:8080/metrics?labels=a,b"}, v)
	})

	t.Run("skips an empty value", func(t *testing.T) {
		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.Var(&v, "url", "")

		require.NoError(t, fs.Parse([]string{"-url="}))
		require.Nil(t, v)
	})

	t.Run("skips empty values but keeps the rest", func(t *testing.T) {
		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.Var(&v, "url", "")

		require.NoError(t, fs.Parse([]string{
			"-url=",
			"-url=http://127.0.0.1:8080/metrics",
			"-url=",
		}))
		require.Equal(t, FlagStringSliceValue{"http://127.0.0.1:8080/metrics"}, v)
	})
}

func TestStringSliceVarEnv(t *testing.T) {
	t.Run("reads the base and numbered env vars in order", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")
		t.Setenv("DP_TEST_URL1", "http://127.0.0.1:9090/metrics")
		t.Setenv("DP_TEST_URL2", "http://127.0.0.1:7070/metrics")

		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		StringSliceVar(fs, &v, "url", "DP_TEST_URL", "")

		require.Equal(t, FlagStringSliceValue{
			"http://127.0.0.1:8080/metrics",
			"http://127.0.0.1:9090/metrics",
			"http://127.0.0.1:7070/metrics",
		}, v)
	})

	t.Run("the base env var alone still works", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")

		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		StringSliceVar(fs, &v, "url", "DP_TEST_URL", "")

		require.Equal(t, FlagStringSliceValue{"http://127.0.0.1:8080/metrics"}, v)
	})

	t.Run("flags append to env var values", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")

		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		StringSliceVar(fs, &v, "url", "DP_TEST_URL", "")
		require.NoError(t, fs.Parse([]string{"-url=http://127.0.0.1:9090/metrics"}))

		require.Equal(t, FlagStringSliceValue{
			"http://127.0.0.1:8080/metrics",
			"http://127.0.0.1:9090/metrics",
		}, v)
	})
}
