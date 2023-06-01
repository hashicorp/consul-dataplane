package envoy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func TestProxy(t *testing.T) {
	bootstrapConfig := []byte(`hello world`)

	// This test checks that we're starting the Envoy process with the correct
	// arguments and that it is able to read the config we provide. It does so
	// by using a mock program called fake-envoy (in the testdata directory)
	// which writes to an output file we specify.
	outputPath := testOutputPath()
	t.Cleanup(func() { _ = os.Remove(outputPath) })

	p, err := NewProxy(ProxyConfig{
		Logger:          hclog.New(&hclog.LoggerOptions{Level: hclog.Warn, Output: io.Discard}),
		EnvoyLogOutput:  io.Discard,
		ExecutablePath:  "testdata/fake-envoy",
		ExtraArgs:       []string{"--test-output", outputPath},
		BootstrapConfig: bootstrapConfig,
	})
	require.NoError(t, err)
	require.NoError(t, p.Run(context.Background()))
	t.Cleanup(func() { _ = p.Stop() })

	// Read the output written by fake-envoy. It might take a while, so poll the
	// file for a couple of seconds.
	var output struct {
		Args       []byte
		ConfigData []byte
	}
	require.Eventually(t, func() bool {
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Logf("failed to read output file: %v", err)
			return false
		}
		if err := json.Unmarshal(outputBytes, &output); err != nil {
			t.Logf("failed to unmarshal output file: %v", err)
			return false
		}
		return true
	}, 2*time.Second, 50*time.Millisecond)

	// Check that fake-envoy was able to read the config from the pipe.
	require.Equal(t, bootstrapConfig, output.ConfigData)

	// Check that we're correctly configuring the log level.
	require.Contains(t, string(output.Args), "--log-level warn")

	// Check that we're disabling hot restarts.
	require.Contains(t, string(output.Args), "--disable-hot-restart")

	// Check the process is still running.
	require.NoError(t, p.cmd.Process.Signal(syscall.Signal(0)))

	// Ensure Stop kills and reaps the process.
	require.NoError(t, p.Stop())

	require.Eventually(t, func() bool {
		return p.cmd.Process.Signal(syscall.Signal(0)) == os.ErrProcessDone
	}, 2*time.Second, 50*time.Millisecond)
}

func TestProxy_Crash(t *testing.T) {
	outputPath := testOutputPath()
	t.Cleanup(func() { _ = os.Remove(outputPath) })

	p, err := NewProxy(ProxyConfig{
		ExecutablePath:  "testdata/fake-envoy",
		ExtraArgs:       []string{"--test-output", outputPath},
		BootstrapConfig: []byte(`hello world`),
		EnvoyLogOutput:  io.Discard,
	})
	require.NoError(t, err)
	require.NoError(t, p.Run(context.Background()))
	t.Cleanup(func() { _ = p.Stop() })

	// Check the process is running.
	require.NoError(t, p.cmd.Process.Signal(syscall.Signal(0)))

	// Kill it!
	require.NoError(t, p.cmd.Process.Kill())

	select {
	case <-p.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Exited channel to be closed")
	}

	require.Equal(t, stateStopped, p.getState())
}

func TestProxy_ContextDone(t *testing.T) {
	outputPath := testOutputPath()
	t.Cleanup(func() { _ = os.Remove(outputPath) })

	p, err := NewProxy(ProxyConfig{
		ExecutablePath:  "testdata/fake-envoy",
		ExtraArgs:       []string{"--test-output", outputPath},
		BootstrapConfig: []byte(`hello world`),
		EnvoyLogOutput:  io.Discard,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, p.Run(ctx))
	t.Cleanup(func() { _ = p.Stop() })

	// Check the process is running.
	require.NoError(t, p.cmd.Process.Signal(syscall.Signal(0)))

	// Finish the context
	cancel()

	select {
	case <-p.Exited():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Exited channel to be closed")
	}

	require.Equal(t, stateStopped, p.getState())
}

func testOutputPath() string {
	return filepath.Join(
		os.TempDir(),
		fmt.Sprintf("test-output-%x.json", time.Now().UnixNano()+int64(os.Getpid())),
	)
}

func TestProxy_OverridingLoggerAndExtraArgs(t *testing.T) {
	bootstrapConfig := []byte(`hello world`)

	// Test checks we are starting proxy with only one argument of log-level
	// When log-level is present in both ProxyConfig and ExtraArgs
	// We override the log-level with that of ExtraArgs
	outputPath := testOutputPath()
	t.Cleanup(func() { _ = os.Remove(outputPath) })

	p, err := NewProxy(ProxyConfig{
		Logger:            hclog.New(&hclog.LoggerOptions{Level: hclog.Warn, Output: io.Discard}),
		EnvoyErrorStream:  io.Discard,
		EnvoyOutputStream: io.Discard,
		ExecutablePath:    "testdata/fake-envoy",
		ExtraArgs:         []string{"--test-output", outputPath, "--log-level", "debug"},
		BootstrapConfig:   bootstrapConfig,
	})
	require.NoError(t, err)
	require.NoError(t, p.Run(context.Background()))
	t.Cleanup(func() { _ = p.Stop() })

	// Read the output written by fake-envoy. It might take a while, so poll the
	// file for a couple of second
	var output struct {
		Args       []byte
		ConfigData []byte
	}
	require.Eventually(t, func() bool {
		outputBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Logf("failed to read output file: %v", err)
			return false
		}
		if err := json.Unmarshal(outputBytes, &output); err != nil {
			t.Logf("failed to unmarshal output file: %v", err)
			return false
		}
		return true
	}, 2*time.Second, 50*time.Millisecond)

	// Check that fake-envoy was able to read the config from the pipe.
	require.Equal(t, bootstrapConfig, output.ConfigData)

	// Check that we're correctly configuring the log level.
	require.Contains(t, string(output.Args), "--log-level debug")

	// Check that we're removing the logger in Proxy config and overriding it with extra args
	require.NotContains(t, string(output.Args), "--log-level warn")

	// Check that we're disabling hot restarts.
	require.Contains(t, string(output.Args), "--disable-hot-restart")

	// Check the process is still running.
	require.NoError(t, p.cmd.Process.Signal(syscall.Signal(0)))

	// Ensure Stop kills and reaps the process.
	require.NoError(t, p.Stop())

	require.Eventually(t, func() bool {
		return p.cmd.Process.Signal(syscall.Signal(0)) == os.ErrProcessDone
	}, 2*time.Second, 50*time.Millisecond)
}
