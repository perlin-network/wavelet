#!/bin/bash
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
