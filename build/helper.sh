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
    echo "Building wavelet and wctl for ${os_arch}"
    GOOS=${OS}
    GOARCH=${ARCH}

    go build \
        -o ${BUILD_BIN}/${OS}-${ARCH}/wavelet \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/params.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/params.GoVersion=${GO_VERSION}" \
        cmd/wavelet/main.go

    go build \
        -o ${BUILD_BIN}/${OS}-${ARCH}/wctl \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/params.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/params.GoVersion=${GO_VERSION}" \
        cmd/wctl/main.go

    if [ -d "cmd/lens/statik" ]; then
        # only build lens if the cmd/lens/build/statik.sh was ran
        go build \
            -o ${BUILD_BIN}/${OS}-${ARCH}/lens \
            -ldflags "-s -w \
                -X ${PROJ_DIR}/params.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/params.GoVersion=${GO_VERSION}" \
            cmd/lens/main.go
    fi

done
