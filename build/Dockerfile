# Dockerfile for automated build and testing

FROM golang:1.11

# If OS_ARCH is not passed, build linux-amd64 by default
ARG OS_ARCH=linux-amd64
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

# build wavelet and pctl for each target
RUN bash -c "for os_arch in \$( echo \${OS_ARCH} | tr \",\" \" \" ); do \
        IFS=\"-\" read -r -a array <<< \${os_arch}; \
        OS=\${array[0]}; \
        ARCH=\${array[1]}; \
        echo \"Building wavelet and wctl for \${os_arch}\"; \
        . /tmp/build_vars.sh && \
        GOOS=\${OS} \
        GOARCH=\${ARCH} \
        go build \
            -o \${BUILD_BIN}/\${OS}-\${ARCH}/wavelet \
            -ldflags \"-s -w \
                -X \${PROJ_DIR}/cmd/utils.GitCommit=\${GIT_COMMIT} \
                -X \${PROJ_DIR}/cmd/utils.GoVersion=\${GO_VERSION}\" \
            cmd/wavelet/main.go && \
        go build \
            -o \${BUILD_BIN}/\${OS}-\${ARCH}/wctl \
            -ldflags \"-s -w \
                    -X \${PROJ_DIR}/cmd/utils.GitCommit=\${GIT_COMMIT} \
                    -X \${PROJ_DIR}/cmd/utils.GoVersion=\${GO_VERSION}\" \
            cmd/wctl/main.go; \
    done;"