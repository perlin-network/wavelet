#!/bin/bash
#
# This script compiles wavelet and pctl in a Docker container for the specified environments.
set -eu

# get platform
PLATFORM='unknown'
unamestr=`uname`
if [[ "$unamestr" == 'Linux' ]]; then
   PLATFORM='linux'
elif [[ "$unamestr" == 'Darwin' ]]; then
   PLATFORM='darwin'
elif [[ "$unamestr" == 'Windows' ]]; then
   PLATFORM='windows'
fi

OPTIND=1
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
HOST_BUILD_BIN="${SCRIPT_DIR}/bin/pkg"
OS_ARCH="${PLATFORM}-amd64"
CLEAR_BUILDS=false

function show_help {
    echo "Usage: build.sh [-h] [-d] [-a arch] [-o output]"
    echo "    -h    Display this help message."
    echo "    -a    OS and architecture (default: ${OS_ARCH}). You can specify multiple values (e.g. linux-amd64,linux-armv7,linux-arm64,darwin-amd64,windows-amd64)."
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

mkdir -p ${HOST_BUILD_BIN}

docker run \
    --rm \
    --user $(id -u):$(id -g) \
    --env GOCACHE="off" \
    --env BUILD_BIN="/output" \
    --env PROJ_DIR="github.com/perlin-network/wavelet" \
    --env OS_ARCH=${OS_ARCH} \
    --mount type=bind,source="${SCRIPT_DIR}/..",target="/go/src/github.com/perlin-network/wavelet" \
    --mount type=bind,source="${HOST_BUILD_BIN}",target="/output" \
    --workdir "/go/src/github.com/perlin-network/wavelet" \
    golang:1.11 \
        bash build/helper.sh

echo "Done building."
