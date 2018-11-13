package params

import "fmt"

const (
	// VersionMajor is major version component of the current release
	VersionMajor = 0
	// VersionMinor is minor version component of the current release
	VersionMinor = 0
	// VersionPatch is patch version component of the current release
	VersionPatch = 2
	// VersionMeta is append to the version string
	VersionMeta = "testnet"
)

// variables set via linker flags
var (
	GitCommit = "unset"
	GoVersion = "unset"
	OSArch    = "unset"
)

// Version holds the textual version string.
var Version = func() string {
	return fmt.Sprintf("v%d.%d.%d-%s", VersionMajor, VersionMinor, VersionPatch, VersionMeta)
}()
