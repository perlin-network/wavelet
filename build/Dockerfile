# Dockerfile for automated build and testing

FROM golang:1.11

ENV PROJ_DIR=github.com/perlin-network/wavelet
ENV REPO_DIR=/go/src/${PROJ_DIR}
ENV BUILD_BIN=/pkg

COPY . ${REPO_DIR}
WORKDIR ${REPO_DIR}

# figure out build time variables to reuse later
RUN echo "# some env variables \n\
export GIT_COMMIT=$(git rev-parse --short HEAD) \n\
export GO_VERSION=$(go version | awk '{print $3}') \n\
" > /tmp/build_vars.sh

# compile the wavelet executable, inject variables at build time
RUN . /tmp/build_vars.sh && \
    CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64 \
    go build \
        -o ${BUILD_BIN}/linux-amd64/wavelet \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
        cmd/wavelet/main.go && \
    ${BUILD_BIN}/linux-amd64/wavelet -version
# RUN . /tmp/build_vars.sh && \
#     GOOS=linux \
#     GOARCH=arm \
#     GOARM=6 \
#     go build \
#         -o ${BUILD_BIN}/linux-arm6/wavelet \
#         -ldflags "-s -w \
#             -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
#             -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
#         cmd/wavelet/main.go
# RUN . /tmp/build_vars.sh && \
#     GOOS=linux \
#     GOARCH=arm \
#     GOARM=7 \
#     go build \
#         -o ${BUILD_BIN}/linux-arm7/wavelet \
#         -ldflags "-s -w \
#             -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
#             -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
#         cmd/wavelet/main.go
# RUN . /tmp/build_vars.sh && \
#     GOOS=linux \
#     GOARCH=arm64 \
#     go build \
#         -o ${BUILD_BIN}/linux-arm64/wavelet \
#         -ldflags "-s -w \
#             -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
#             -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
#         cmd/wavelet/main.go
RUN . /tmp/build_vars.sh && \
    CGO_ENABLED=0 \
    GOOS=windows \
    GOARCH=amd64 \
    go build \
        -o ${BUILD_BIN}/win-amd64/wavelet.exe \
        -ldflags "-s -w \
            -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
            -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
        cmd/wavelet/main.go
# RUN . /tmp/build_vars.sh && \
#     GOOS=darwin \
#     GOARCH=amd64 \
#     go build \
#         -o ${BUILD_BIN}/darwin-amd64/wavelet \
#         -ldflags "-s -w \
#             -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
#             -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
#         cmd/wavelet/main.go

# compile the wctl exec
RUN . /tmp/build_vars.sh && \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build \
        -o ${BUILD_BIN}/linux-amd64/wctl \
        -ldflags "-s -w \
                -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
        cmd/wctl/main.go && \
    ${BUILD_BIN}/linux-amd64/wctl -version
RUN . /tmp/build_vars.sh && \
    CGO_ENABLED=0 \
    GOOS=windows \
    GOARCH=amd64 \
    go build \
        -o ${BUILD_BIN}/win-amd64/wctl \
        -ldflags "-s -w \
                -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
                -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
        cmd/wctl/main.go
# RUN . /tmp/build_vars.sh && \
#     GOOS=darwin \
#     GOARCH=amd64 \
#     go build \
#         -o ${BUILD_BIN}/darwin-amd64/wctl \
#         -ldflags "-s -w \
#                 -X ${PROJ_DIR}/cmd/utils.GitCommit=${GIT_COMMIT} \
#                 -X ${PROJ_DIR}/cmd/utils.GoVersion=${GO_VERSION}" \
#         cmd/wctl/main.go

# run the tests to make sure everything passes
RUN go test -coverprofile=coverage.txt -covermode=atomic -timeout 300s -v -bench -race ./...
