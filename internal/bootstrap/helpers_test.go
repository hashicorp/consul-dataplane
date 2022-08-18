package bootstrap

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testSetAndResetEnv sets the env vars passed as KEY=value strings in the
// current ENV and returns a func() that will undo it's work at the end of the
// test for use with defer.
func testSetAndResetEnv(t *testing.T, env []string) func() {
	old := make(map[string]*string)
	for _, e := range env {
		pair := strings.SplitN(e, "=", 2)
		current := os.Getenv(pair[0])
		if current != "" {
			old[pair[0]] = &current
		} else {
			// save it as a nil so we know to remove again
			old[pair[0]] = nil
		}
		require.NoError(t, os.Setenv(pair[0], pair[1]))
	}
	// Return a func that will reset to old values
	return func() {
		for k, v := range old {
			if v == nil {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, *v)
			}
		}
	}
}
