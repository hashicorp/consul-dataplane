package version

import (
	_ "embed"
	"fmt"
	"strings"
)

var (
	// The git commit that was compiled. These will be filled in by the
	// compiler.
	GitCommit string

	// The main version number that is being run at the moment.
	// Version must conform to the format expected by github.com/hashicorp/go-version
	// for tests to work.
	// VersionPrerelease is a pre-release marker for the version. If this is "" (empty string)
	// then it means that it is a final release. Otherwise, this is a pre-release
	// such as "dev" (in development), "beta", "rc1", etc.
	// Version and VersionPrerelease info are now being embedded directly from the VERSION file.
	//go:embed VERSION
	fullVersion                   string
	Version, VersionPrerelease, _ = strings.Cut(strings.TrimSpace(fullVersion), "-")
)

// GetHumanVersion composes the parts of the version in a way that's suitable
// for displaying to humans.
func GetHumanVersion() string {
	version := Version
	release := VersionPrerelease

	if release != "" {
		if !strings.HasSuffix(version, "-"+release) {
			// if we tagged a prerelease version then the release is in the version already
			version += fmt.Sprintf("-%s", release)
		}
	}

	if IsFIPS() {
		version = fmt.Sprintf("%s+fips1402", version)
	}

	// Strip off any single quotes added by the git information.
	return strings.ReplaceAll(version, "'", "")
}
