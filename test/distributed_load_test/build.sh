#!/bin/bash -eux

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
IMAGE_NAME="us.gcr.io/wavelet-dev/load-test"

rm -rf $SCRIPT_DIR/__pycache__
cd $SCRIPT_DIR/../..

# Build the Docker images in wavelet root
go mod vendor
docker build -t ${IMAGE_NAME} -f test/distributed_load_test/Dockerfile .