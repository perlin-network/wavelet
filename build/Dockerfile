# Dockerfile for automated build and testing

FROM golang:1.11

# cannot use go modules since one of the dependencies is still private
#ENV GO111MODULE=on

ENV PROJ_DIR=github.com/perlin-network/wavelet
ENV REPO_DIR=/go/src/${PROJ_DIR}
ENV BUILD_BIN=/pkg/linux-amd64/

COPY . ${REPO_DIR}
WORKDIR ${REPO_DIR}

# compile the wavelet executable, inject variables at build time
RUN GIT_COMMIT=$(git rev-parse --short HEAD) && \
    GO_VERSION=$(go version | awk '{print $3}') && \
    go build \
        -o ${BUILD_BIN}/wavelet \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
        cmd/wavelet/main.go && \
    ${BUILD_BIN}/wavelet -version

RUN go build \
    -o ${BUILD_BIN}/wctl \
    -ldflags "-s -w \
            -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
    cmd/wctl/main.go

RUN go test -coverprofile=coverage.txt -covermode=atomic -timeout 300s -v -bench -race ./...
