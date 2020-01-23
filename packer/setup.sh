#!/bin/sh

apt-get update
apt-get install -y unzip git build-essential

WAVELET_DIR=/opt/wavelet

groupadd wavelet
useradd wavelet -g wavelet

mkdir -p $WAVELET_DIR

TMPDIR=$(mktemp -d)
cd $TMPDIR

if [ -z "$WAVELET_BUILD_BRANCH" ]
then
  # Download release
  wget https://github.com/perlin-network/wavelet/releases/download/${WAVELET_VERSION}/wavelet-${WAVELET_VERSION}-testnet-linux-amd64.zip
  unzip wavelet-${WAVELET_VERSION}-testnet-linux-amd64.zip
  
  mv wavelet $WAVELET_DIR
  mv benchmark $WAVELET_DIR
  mv wctl $WAVELET_DIR
else
  # Build from branch

  # Install go
  wget https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz
  tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
  export PATH=$PATH:/usr/local/go/bin

  git clone https://github.com/perlin-network/wavelet
  cd wavelet
  git checkout ${WAVELET_BUILD_BRANCH}

  GOOS=linux GOARCH=amd64 go build -o $WAVELET_DIR/wavelet ./cmd/wavelet
  GOOS=linux GOARCH=amd64 go build -o $WAVELET_DIR/benchmark ./cmd/benchmark
fi

chown -R wavelet $WAVELET_DIR

rm -rf $TMPDIR

