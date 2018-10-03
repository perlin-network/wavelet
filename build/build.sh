#!/bin/bash -eux

OPTIND=1
IMAGE_NAME="perlin/wavelet"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
HOST_BUILD_BIN="${SCRIPT_DIR}/bin"
OS_ARCH="linux-amd64"
CLEAR_BUILDS=false

function show_help {
    echo "Usage: build.sh [-h] [-d] [-a arch] [-o output]"
    echo "    -h    Display this help message."
    echo "    -a    OS and architecture (default: ${OS_ARCH}). You can specify multiple values (e.g. linux-amd64,darwin-amd64,windows-amd64)."
    echo "    -d    Delete old builds (default: ${CLEAR_BUILDS})."
    echo "    -o    Binary output directory (default: ${HOST_BUILD_BIN})."
}

while getopts "h?a:o:d" opt; do
    case "$opt" in
    h|\?)
        show_help
        exit 0
        ;;
    a)  OS_ARCH="$OPTARG"
        ;;
    d)  CLEAR_BUILDS=true
        ;;
    o)  HOST_BUILD_BIN="$OPTARG"
        ;;
    esac
done
shift $((OPTIND-1))

cd ${SCRIPT_DIR}/..

# add the go vendor dependencies since perlin-network/graph is still private, need host credentials
go mod vendor

# clear out old builds
if [ ${CLEAR_BUILDS} = true ] && [ -d ${HOST_BUILD_BIN} ]; then
    rm -rf ${HOST_BUILD_BIN}
fi

if [ ! -d ${HOST_BUILD_BIN} ]; then
    mkdir -p ${HOST_BUILD_BIN}
fi

# make a workspace to compile and test
docker build \
    --tag ${IMAGE_NAME} \
    --file build/Dockerfile \
    --build-arg OS_ARCH=${OS_ARCH} \
    $(pwd)

# copy the binaries to the host bin directory
CONTAINER_BUILD_BIN=$(docker run --rm ${IMAGE_NAME} bash -c "echo \$BUILD_BIN")
docker run \
    --rm \
    --mount type=bind,source="${HOST_BUILD_BIN}",target="/host-bin" \
    ${IMAGE_NAME} \
    bash -c "cp -r ${CONTAINER_BUILD_BIN}/ /host-bin/ && \
        chown $(id -u):$(id -g) /host-bin/*"

echo "Done building."
