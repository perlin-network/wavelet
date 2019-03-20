#!/bin/bash
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

# loop through each architecture and build to an output
for os_arch in $( echo ${OS_ARCH} | tr "," " " ); do
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

    go build \
        -a \
        -o ${BUILD_BIN}/${OS}-${ARCH}/wavelet${BINARY_POSTFIX} \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/sys.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/sys.GoVersion=${GO_VERSION} \
            -X ${PROJ_DIR}/sys.OSArch=${os_arch}" \
        cmd/wavelet/main.go

    go build \
        -a \
        -o ${BUILD_BIN}/${OS}-${ARCH}/wctl${BINARY_POSTFIX} \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/sys.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/sys.GoVersion=${GO_VERSION} \
            -X ${PROJ_DIR}/sys.OSArch=${os_arch}" \
        cmd/wctl/main.go

    go build \
        -a \
        -o ${BUILD_BIN}/${OS}-${ARCH}/benchmark${BINARY_POSTFIX} \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/sys.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/sys.GoVersion=${GO_VERSION} \
            -X ${PROJ_DIR}/sys.OSArch=${os_arch}" \
        cmd/benchmark/*.go

done