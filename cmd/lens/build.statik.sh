#!/bin/bash
set -eu

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
GIT_BRANCH=${GIT_BRANCH-"master"}

cd ${SCRIPT_DIR}
rm -rf lens statik

# clone the lens repo
git clone --single-branch --branch ${GIT_BRANCH} git@github.com:perlin-network/lens.git

# get the git hash of the lens repo and put it into statik/version.go
LENS_GIT_VERSION=$(git -C lens rev-parse --short HEAD)
mkdir -p statik
echo "package statik

const LensGitVersion = \"${LENS_GIT_VERSION}\"
" > statik/version.go

# build the prod version into the lens/build dir
bash lens/scripts/build.prod.sh

# use statik to convert the prod files into a go file in the statik dir
docker run \
    --rm \
    --user $(id -u):$(id -g) \
    --env GOCACHE="off" \
    --mount type=bind,source="${SCRIPT_DIR}",target="/lens" \
    --workdir "/lens" \
    golang:1.11 \
        bash -c "go get github.com/rakyll/statik && \
            statik -f -src=lens/build -p statik -dest ."

# clean up
rm -rf ${SCRIPT_DIR}/lens

echo "Done building."
