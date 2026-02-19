// Copyright IBM Corp. 2022, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type DurationTestInput struct {
	Key1     string    `json:"key1"`
	Key2     string    `json:"key2"`
	Duration *Duration `json:"duration,omitempty"`
}

func TestUnmarshalForStringBasedDurationInput(t *testing.T) {
	data := `
	{
		"key1": "value1",
		"key2": "value2",
		"duration": "100s"
	}
	`

	d, err := time.ParseDuration("100s")
	require.NoError(t, err)

	expectedDuration := &Duration{
		Duration: d,
	}

	var durationTestInput *DurationTestInput
	err = json.Unmarshal([]byte(data), &durationTestInput)
	require.NoError(t, err)

	require.NotNil(t, durationTestInput)
	require.NotNil(t, durationTestInput.Duration)
	require.Equal(t, expectedDuration.Duration, durationTestInput.Duration.Duration)
}

func TestUnmarshalForFloatBasedDurationInput(t *testing.T) {
	data := `
	{
		"key1": "value1",
		"key2": "value2",
		"duration": 4.5
	}
	`

	in := 4.5
	expectedDuration := &Duration{
		Duration: time.Duration(in),
	}

	var durationTestInput *DurationTestInput
	err := json.Unmarshal([]byte(data), &durationTestInput)
	require.NoError(t, err)

	require.NotNil(t, durationTestInput)
	require.NotNil(t, durationTestInput.Duration)
	require.Equal(t, expectedDuration.Duration, durationTestInput.Duration.Duration)
}
