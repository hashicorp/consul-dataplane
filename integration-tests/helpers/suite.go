package helpers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

type SuiteOptions struct {
	// OutputDir is the directory artifacts will be written to. It can be configured
	// using the -output-dir flag.
	OutputDir string

	// DisableReaper controls whether the container reaper is enabled. It can be
	// configured using the -disable-reaper flag.
	//
	// See: https://hub.docker.com/r/testcontainers/ryuk
	DisableReaper bool

	// ServerImage is the container image reference for the Consul server. It can
	// be configured using the -server-image flag.
	ServerImage string

	// DataplaneImage is the container image reference for Consul Dataplane. It
	// can be configured using the -dataplane-image flag.
	DataplaneImage string
}

// Suite handles the lifecycle of resources (e.g. containers) created during a
// test, and writes artifacts to disk when the test finishes.
type Suite struct {
	// Name is used as a prefix in container and volume names.
	Name string

	opts SuiteOptions

	mu        sync.Mutex
	artifacts map[string][]byte
	volume    *Volume
}

func NewSuite(t *testing.T, opts SuiteOptions) *Suite {
	suite := &Suite{
		Name:      fmt.Sprintf("int-%d", time.Now().UnixNano()),
		opts:      opts,
		artifacts: make(map[string][]byte),
	}

	t.Cleanup(func() {
		suite.cleanup(t)
	})

	return suite
}

type ContainerRequest = testcontainers.ContainerRequest

// RunContainer runs a container, capturing its logs as artifacts, and stopping
// the container when the test finishes.
func (s *Suite) RunContainer(t *testing.T, name string, captureLogs bool, req ContainerRequest) *Container {
	t.Helper()

	ctx := s.Context(t)

	req.Name = fmt.Sprintf("%s-%s", s.Name, name)
	req.AutoRemove = false
	req.SkipReaper = s.opts.DisableReaper

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	logs := &logConsumer{}
	container.FollowOutput(logs)

	require.NoError(t, container.StartLogProducer(ctx))

	t.Cleanup(func() {
		if err := container.StopLogProducer(); err != nil {
			t.Logf("failed to stop log producer: %v\n", err)
		}

		if captureLogs {
			s.CaptureArtifact(
				fmt.Sprintf("%s.log", name),
				logs.bytes(),
			)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		state, err := container.State(ctx)
		if err != nil {
			t.Logf("failed to get container state (%s): %v\n", req.Name, err)
			return
		}

		if !state.Running {
			return
		}

		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container (%s): %v\n", req.Name, err)
		}
	})

	hostIP, err := container.Host(ctx)
	require.NoError(t, err)

	containerIP, err := container.ContainerIP(ctx)
	require.NoError(t, err)

	hostMappedPorts := make(map[nat.Port]int, len(req.ExposedPorts))
	for _, portString := range req.ExposedPorts {
		port := nat.Port(portString)
		hostPort, err := container.MappedPort(ctx, port)
		require.NoError(t, err)
		hostMappedPorts[port] = hostPort.Int()
	}

	return &Container{
		Name:        req.Name,
		Container:   container,
		HostIP:      hostIP,
		ContainerIP: containerIP,
		MappedPorts: hostMappedPorts,
	}
}

// Context returns a context that will be canceled when the test finishes.
func (s *Suite) Context(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// CaptureArtifact stores the given data to be written to disk when the test
// finishes.
func (s *Suite) CaptureArtifact(name string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.artifacts[name] = data
}

func (s *Suite) cleanup(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.opts.OutputDir == "" {
		return
	}

	for name, data := range s.artifacts {
		if err := os.WriteFile(filepath.Join(s.opts.OutputDir, name), data, 0660); err != nil {
			t.Logf("failed to write artifact %s: %v", name, err)
		}
	}
}

// Volume returns a Docker volume that can be used to share files between
// containers. You can also add files from the host using WriteFile. The
// volume will be deleted when the test finishes.
func (s *Suite) Volume(t *testing.T) *Volume {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.volume == nil {
		docker, _, _, err := testcontainers.NewDockerClient()
		require.NoError(t, err)

		v, err := docker.VolumeCreate(
			s.Context(t),
			volume.VolumeCreateBody{
				Name: fmt.Sprintf("%s-volume", s.Name),
			},
		)
		require.NoError(t, err)

		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := docker.VolumeRemove(ctx, v.Name, true); err != nil {
				t.Logf("failed to remove volume: %v", err)
			}
		})

		s.volume = &Volume{Volume: v, suite: s}
	}

	return s.volume
}

type Container struct {
	testcontainers.Container

	Name        string
	HostIP      string
	ContainerIP string
	MappedPorts map[nat.Port]int
}

// Network returns the container's network that can be used to join other
// containers to it.
func (c *Container) Network() container.NetworkMode {
	return container.NetworkMode(fmt.Sprintf("container:%s", c.Name))
}

// ContainerLogs returns the container's logs.
func (c *Container) ContainerLogs(t *testing.T) string {
	rc, err := c.Logs(context.Background())
	defer rc.Close()

	require.NoError(t, err)
	out, err := io.ReadAll(rc)
	require.NoError(t, err)

	return string(out)
}

type Volume struct {
	types.Volume

	suite     *Suite
	mu        sync.Mutex
	container *Container
}

// WriteFile adds a file to the Volume using a "pause" container.
func (v *Volume) WriteFile(t *testing.T, name string, contents []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()

	const mountPoint = "/data"

	if v.container == nil {
		v.container = v.suite.RunContainer(t, "volume", false, ContainerRequest{
			Image: pauseContainerImage,
			Mounts: []testcontainers.ContainerMount{
				testcontainers.VolumeMount(v.Name, mountPoint),
			},
		})
	}

	ctx := v.suite.Context(t)
	err := v.container.CopyToContainer(ctx, contents, filepath.Join(mountPoint, name), 0444)
	require.NoError(t, err)
}

type logConsumer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logConsumer) Accept(l testcontainers.Log) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buf.Write(l.Content)
}

func (c *logConsumer) bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.buf.Bytes()
}
