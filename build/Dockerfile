# Dockerfile for automated build and testing

FROM golang:1.11

# cannot use go modules since one of the dependencies is still private
#ENV GO111MODULE=on
ENV REPO_DIR=/go/src/github.com/perlin-network/wavelet
ENV BUILD_BIN=/pkg/linux-amd64/

COPY . ${REPO_DIR}
WORKDIR ${REPO_DIR}

# compile the wavelet executable, inject variables at build time
RUN GIT_COMMIT=$(git rev-parse --short HEAD) && \
    BUILT_TIME=$(date '+%a %b %d %H:%M:%S %Y') && \
    GO_VERSION=$(go version | awk '{print $3}') && \
    go build \
    -o ${BUILD_BIN}/wavelet \
    -ldflags "-s -w \
        -X \${REPO_DIR}/cmd/utils/version.GitCommit=\${GIT_COMMIT} \
        -X \${REPO_DIR}/cmd/utils/version.BuiltTime=\${BUILT_TIME} \
        -X \${REPO_DIR}/cmd/utils/version.GoVersion=\${GO_VERSION}" \
    cmd/wavelet/main.go

#RUN go build -o ${BUILD_BIN}/plctl cmd/plctl/plctl.go

RUN go test -coverprofile=coverage.txt -covermode=atomic -timeout 300s -v -bench -race ./...
