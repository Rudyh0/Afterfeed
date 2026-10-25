// Package version holds build information, set at build time with -ldflags.
package version

var (
	// Version is the release version, for example "v0.1.0".
	Version = "dev"
	// Commit is the git commit the binary was built from.
	Commit = "none"
	// Date is the build date in RFC 3339 format.
	Date = "unknown"
)
