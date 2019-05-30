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

# get platform
OS_PLATFORM='unknown'
unamestr=`uname`
if [[ "$unamestr" == 'Linux' ]]; then
   OS_PLATFORM='linux'
elif [[ "$unamestr" == 'Darwin' ]]; then
   OS_PLATFORM='darwin'
elif [[ "$unamestr" == 'Windows' ]]; then
   OS_PLATFORM='windows'
fi

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
BIN_DIR="${SCRIPT_DIR}/../build/bin/pkg"
CMD_WAVELET_DIR="${SCRIPT_DIR}/../cmd/wavelet"
VERSION=$(${BIN_DIR}/${OS_PLATFORM}-amd64/wavelet -v | grep "^Version:" | awk '{print $2}')

# clean up old zip files
rm ${SCRIPT_DIR}/bin/*.zip || true

# loop through all the platforms and create an archive
cd ${SCRIPT_DIR}
for PLATFORM_DIR in ${BIN_DIR}/*; do
    PLATFORM=$(basename ${PLATFORM_DIR})
    echo "Archiving platform ${PLATFORM}"

    # copy over the auxilary files
    cp -R ${CMD_WAVELET_DIR}/config ${BIN_DIR}/${PLATFORM}

    cd ${BIN_DIR}/${PLATFORM}
    zip -r "${SCRIPT_DIR}/../build/bin/wavelet-${VERSION}-${PLATFORM}.zip" *
done

echo "Release done"
