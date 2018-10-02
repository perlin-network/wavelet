#!/bin/bash
set -eu

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
IMAGE_NAME="perlin/wavelet"
HOST_BUILD_BIN="${SCRIPT_DIR}/_bin"

cd ${SCRIPT_DIR}/..

# add the go vendor dependencies since perlin-network/graph is still private, need host credentials
go mod vendor

# clear out old builds
rm -rf ${HOST_BUILD_BIN}

# make a workspace to compile and test
docker build \
    --tag ${IMAGE_NAME} \
    --file build/Dockerfile \
    $(pwd)

# copy the binaries to the host bin directory
mkdir -p ${HOST_BUILD_BIN}
CONTAINER_BUILD_BIN=$(docker run --rm ${IMAGE_NAME} bash -c "echo \$BUILD_BIN")
docker run \
    --rm \
    --mount type=bind,source="${HOST_BUILD_BIN}",target="/host-bin" \
    ${IMAGE_NAME} \
    bash -c "cp -r ${CONTAINER_BUILD_BIN}/ /host-bin/ && \
        chown $(id -u):$(id -g) /host-bin/*"

echo "Done building."
