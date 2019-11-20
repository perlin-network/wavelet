#!/bin/bash
# Copyright (c) 2019 Perlin
#
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in
# the Software without restriction, including without limitation the rights to
# use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
# the Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
# IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

set -eu

# This script loops compiles all the binaries and puts them into a common folder.
# Should run this in the wavelet root directory.
# Params:
#   OS_ARCH     - OS and architecture. Can specify multiple values (e.g. linux-amd64,linux-armv7,linux-arm64,darwin-amd64,windows-amd64).
#   BUILD_BIN   - output directory, should be build/bin/pkg
#   PROJ_DIR    - go package directory, should be github.com/perlin-network/wavelet

# common variables
GIT_COMMIT=$(git rev-parse --short HEAD)
GO_VERSION=$(go version | awk '{print $3}')
BUILD_NETWORK="${BUILD_NETWORK:-testnet}"

set -e

# loop through each architecture and build to an output
export IFS="," 
for os_arch in ${OS_ARCH}; do
    OS=$(echo "${os_arch}" | cut -d- -f1)
    ARCH=$(echo "${os_arch}" | cut -d- -f2)

    echo "Building binaries for ${os_arch}."

    export GOOS=${OS}

    if [[ "${ARCH}" == 'armv7' ]]; then
        export GOARCH='arm'
        export GOARM=7
    else
        export GOARCH=${ARCH}
    fi

    BINARY_POSTFIX=""
    if [[ "${GOOS}" == 'windows' ]]; then
        BINARY_POSTFIX=".exe"
    fi

    (
        cd cmd/wavelet || exit 1
        CGO_ENABLED=0 go build \
            -a \
            -o ${BUILD_BIN}/${OS}-${ARCH}/wavelet${BINARY_POSTFIX} \
            -ldflags "\
                -X ${PROJ_DIR}/sys.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/sys.GoVersion=${GO_VERSION} \
                -X ${PROJ_DIR}/sys.OSArch=${os_arch} \
                -X ${PROJ_DIR}/sys.VersionMeta=${BUILD_NETWORK} \
                -X ${PROJ_DIR}/sys.GoExe=${BINARY_POSTFIX}" \
            .
    )

    (
exit 0
        cd cmd/wctl || exit 1
        CGO_ENABLED=0 go build \
            -a \
            -o ${BUILD_BIN}/${OS}-${ARCH}/wctl${BINARY_POSTFIX} \
            -ldflags " \
                -X ${PROJ_DIR}/sys.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/sys.GoVersion=${GO_VERSION} \
                -X ${PROJ_DIR}/sys.OSArch=${os_arch} \
                -X ${PROJ_DIR}/sys.VersionMeta=${BUILD_NETWORK} \
                -X ${PROJ_DIR}/sys.GoExe=${BINARY_POSTFIX}" \
            .
    )

    (
        cd cmd/benchmark || exit 1
        CGO_ENABLED=0 go build \
            -a \
            -o ${BUILD_BIN}/${OS}-${ARCH}/benchmark${BINARY_POSTFIX} \
            -ldflags "\
                -X ${PROJ_DIR}/sys.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/sys.GoVersion=${GO_VERSION} \
                -X ${PROJ_DIR}/sys.OSArch=${os_arch} \
                -X ${PROJ_DIR}/sys.VersionMeta=${BUILD_NETWORK} \
                -X ${PROJ_DIR}/sys.GoExe=${BINARY_POSTFIX}" \
            .
    )

done
