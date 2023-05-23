//go:build !fips

package version

// IsFIPS returns true if consul-dataplane is operating in FIPS-140-2 mode.
func IsFIPS() bool {
	return false
}

func GetFIPSInfo() string {
	return ""
}
