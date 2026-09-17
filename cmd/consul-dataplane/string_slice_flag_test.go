// Copyright IBM Corp. 2022, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"flag"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlagStringSliceValue(t *testing.T) {
	newFlagSet := func(v *FlagStringSliceValue) *flag.FlagSet {
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		fs.Var(v, "url", "")
		return fs
	}

	t.Run("accumulates repeated flags in order", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse([]string{
			"-url=http://127.0.0.1:8080/metrics",
			"-url=http://127.0.0.1:9090/admin/metrics",
			"-url", "http://127.0.0.1:7070/metrics",
		}))
		require.Equal(t, []string{
			"http://127.0.0.1:8080/metrics",
			"http://127.0.0.1:9090/admin/metrics",
			"http://127.0.0.1:7070/metrics",
		}, v.Values())
		require.True(t, v.IsSet())
	})

	t.Run("a single flag yields a single value", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse([]string{"-url=http://127.0.0.1:8080/metrics"}))
		require.Equal(t, []string{"http://127.0.0.1:8080/metrics"}, v.Values())
		require.True(t, v.IsSet())
	})

	t.Run("unset stays empty and unsupplied", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse(nil))
		require.Empty(t, v.Values())
		require.False(t, v.IsSet(), "an absent flag must not override lower-precedence configuration")
	})

	t.Run("an explicit empty value is recorded as supplied", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse([]string{"-url="}))
		require.Empty(t, v.Values())
		require.True(t, v.IsSet(), "an explicit empty flag must override lower-precedence configuration")
	})

	t.Run("an empty flag clears previously accumulated values", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse([]string{
			"-url=http://127.0.0.1:8080/metrics",
			"-url=",
		}))
		require.Empty(t, v.Values())
		require.True(t, v.IsSet(), "presence bit must still be set after clearing")
	})

	t.Run("values after an empty flag are accumulated normally", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse([]string{
			"-url=",
			"-url=http://127.0.0.1:8080/metrics",
		}))
		require.Equal(t, []string{"http://127.0.0.1:8080/metrics"}, v.Values())
		require.True(t, v.IsSet())
	})

	t.Run("a value containing a comma is kept intact", func(t *testing.T) {
		var v FlagStringSliceValue
		require.NoError(t, newFlagSet(&v).Parse([]string{"-url=http://127.0.0.1:8080/metrics?labels=a,b"}))
		require.Equal(t, []string{"http://127.0.0.1:8080/metrics?labels=a,b"}, v.Values())
	})
}

func TestFlagStringSliceValueJSON(t *testing.T) {
	cases := map[string]struct {
		json      string
		expValues []string
		expIsSet  bool
	}{
		"absent field":   {json: `{}`, expValues: nil, expIsSet: false},
		"null":           {json: `{"urls": null}`, expValues: nil, expIsSet: false},
		"empty array":    {json: `{"urls": []}`, expValues: nil, expIsSet: true},
		"single value":   {json: `{"urls": ["http://a/metrics"]}`, expValues: []string{"http://a/metrics"}, expIsSet: true},
		"several values": {json: `{"urls": ["http://a/metrics","http://b/metrics"]}`, expValues: []string{"http://a/metrics", "http://b/metrics"}, expIsSet: true},
		"empty entries":  {json: `{"urls": ["","http://a/metrics"]}`, expValues: []string{"http://a/metrics"}, expIsSet: true},
	}

	for name, c := range cases {
		c := c
		t.Run(name, func(t *testing.T) {
			var target struct {
				URLs FlagStringSliceValue `json:"urls"`
			}
			require.NoError(t, json.Unmarshal([]byte(c.json), &target))
			require.Equal(t, c.expValues, target.URLs.Values())
			require.Equal(t, c.expIsSet, target.URLs.IsSet())
		})
	}

	t.Run("round trips", func(t *testing.T) {
		v := NewFlagStringSliceValue("http://a/metrics", "http://b/metrics")
		data, err := json.Marshal(v)
		require.NoError(t, err)
		require.JSONEq(t, `["http://a/metrics","http://b/metrics"]`, string(data))

		var back FlagStringSliceValue
		require.NoError(t, json.Unmarshal(data, &back))
		require.Equal(t, v.Values(), back.Values())
		require.True(t, back.IsSet())
	})

	t.Run("supplied empty list round trips preserving presence bit", func(t *testing.T) {
		// set=true, values=nil: marshals as [] not null so UnmarshalJSON
		// sets the presence bit and a lower-precedence URL is not used.
		v := NewFlagStringSliceValue()
		data, err := json.Marshal(v)
		require.NoError(t, err)
		require.JSONEq(t, `[]`, string(data))

		var back FlagStringSliceValue
		require.NoError(t, json.Unmarshal(data, &back))
		require.Empty(t, back.Values())
		require.True(t, back.IsSet(), "presence bit must survive a JSON round-trip")
	})
}

func TestStringSliceVarEnv(t *testing.T) {
	t.Run("reads the base and numbered env vars in order", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")
		t.Setenv("DP_TEST_URL1", "http://127.0.0.1:9090/metrics")
		t.Setenv("DP_TEST_URL2", "http://127.0.0.1:7070/metrics")

		var v FlagStringSliceValue
		StringSliceVar(flag.NewFlagSet("", flag.ContinueOnError), &v, "url", "DP_TEST_URL", "")

		require.Equal(t, []string{
			"http://127.0.0.1:8080/metrics",
			"http://127.0.0.1:9090/metrics",
			"http://127.0.0.1:7070/metrics",
		}, v.Values())
	})

	t.Run("the base env var alone still works", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")

		var v FlagStringSliceValue
		StringSliceVar(flag.NewFlagSet("", flag.ContinueOnError), &v, "url", "DP_TEST_URL", "")

		require.Equal(t, []string{"http://127.0.0.1:8080/metrics"}, v.Values())
		require.True(t, v.IsSet())
	})

	t.Run("flags append to env var values", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")

		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		StringSliceVar(fs, &v, "url", "DP_TEST_URL", "")
		require.NoError(t, fs.Parse([]string{"-url=http://127.0.0.1:9090/metrics"}))

		require.Equal(t, []string{
			"http://127.0.0.1:8080/metrics",
			"http://127.0.0.1:9090/metrics",
		}, v.Values())
	})

	t.Run("an explicit empty flag clears env var values", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")

		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		StringSliceVar(fs, &v, "url", "DP_TEST_URL", "")
		require.NoError(t, fs.Parse([]string{"-url="}))

		require.Empty(t, v.Values(), "explicit empty flag must clear env var values")
		require.True(t, v.IsSet(), "presence bit must remain set")
	})

	t.Run("values after an explicit empty flag are accumulated despite env var", func(t *testing.T) {
		t.Setenv("DP_TEST_URL", "http://127.0.0.1:8080/metrics")

		var v FlagStringSliceValue
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		StringSliceVar(fs, &v, "url", "DP_TEST_URL", "")
		require.NoError(t, fs.Parse([]string{
			"-url=",
			"-url=http://127.0.0.1:9090/metrics",
		}))

		require.Equal(t, []string{"http://127.0.0.1:9090/metrics"}, v.Values())
		require.True(t, v.IsSet())
	})
}
