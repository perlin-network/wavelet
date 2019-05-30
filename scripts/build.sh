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

#
# This script compiles wavelet and wctl in a Docker container for the specified environments.
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
HOST_BUILD_BIN="${SCRIPT_DIR}/../build/bin/pkg"
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

# go to project root directory
cd ${SCRIPT_DIR}/..

# clear out old builds
if [ ${CLEAR_BUILDS} = true ] && [ -d ${HOST_BUILD_BIN} ]; then
    rm -rf ${HOST_BUILD_BIN}
fi

mkdir -p ${HOST_BUILD_BIN}

# pull dependencies
go mod vendor

# run the build helper script
BUILD_BIN="${HOST_BUILD_BIN}" \
PROJ_DIR="github.com/perlin-network/wavelet" \
OS_ARCH=${OS_ARCH} \
    bash scripts/helper.sh

echo "Done building."

