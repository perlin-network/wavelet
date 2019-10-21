// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package sys

import "fmt"

const (
	// VersionMajor is major version component of the current release
	VersionMajor = 0
	// VersionMinor is minor version component of the current release
	VersionMinor = 2
	// VersionPatch is patch version component of the current release
	VersionPatch = 0
)

// variables set via linker flags
var (
	GitCommit = "unset"
	GoVersion = "unset"
	OSArch    = "unset"
	GoExe     = ""

	// VersionMeta is append to the version string
	VersionMeta = "testing"
)

// Version holds the textual version string.
var Version = func() string {
	return fmt.Sprintf("v%d.%d.%d-%s", VersionMajor, VersionMinor, VersionPatch, VersionMeta)
}()

var VersionCode = func() uint32 {
	var versionCode uint32

	versionCode = (VersionMajor << 24) | (VersionMinor << 16) | VersionPatch

	return versionCode
}()
