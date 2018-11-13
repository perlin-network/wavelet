#!/bin/bash
set -eu

# common variables
GIT_COMMIT=$(git rev-parse --short HEAD)
GO_VERSION=$(go version | awk '{print $3}')

# loop through each architecture and build to an output
for os_arch in $( echo ${OS_ARCH} | tr "," " " ); do
    IFS="-" read -r -a array <<< ${os_arch}
    OS=${array[0]}
    ARCH=${array[1]}
    echo "Building binaries for ${os_arch}"
    export GOOS=${OS}
    export GOARCH=${ARCH}
    BINARY_POSTFIX=""
    if [[ "${GOOS}" == 'windows' ]]; then
        BINARY_POSTFIX=".exe"
    fi

    go build \
        -a \
        -o ${BUILD_BIN}/${OS}-${ARCH}/wavelet${BINARY_POSTFIX} \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/params.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/params.GoVersion=${GO_VERSION} \
            -X ${PROJ_DIR}/params.OSArch=${os_arch}" \
        cmd/wavelet/main.go

    go build \
        -a \
        -o ${BUILD_BIN}/${OS}-${ARCH}/wctl${BINARY_POSTFIX} \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/params.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/params.GoVersion=${GO_VERSION} \
            -X ${PROJ_DIR}/params.OSArch=${os_arch}" \
        cmd/wctl/main.go

    if [ -d "cmd/lens/statik" ]; then
        # only build lens if the cmd/lens/build/statik.sh was ran
        go build \
            -a \
            -o ${BUILD_BIN}/${OS}-${ARCH}/lens${BINARY_POSTFIX} \
            -ldflags "-s -w \
                -X ${PROJ_DIR}/params.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/params.GoVersion=${GO_VERSION} \
                -X ${PROJ_DIR}/params.OSArch=${os_arch}" \
            cmd/lens/main.go
    fi

done
