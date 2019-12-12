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

FROM golang:1.13-alpine as build
RUN apk add --no-cache git

RUN mkdir /src
COPY go.mod /src/go.mod
COPY go.sum /src/go.sum
RUN (cd /src; go mod download)

ARG GIT_COMMIT
ENV GIT_COMMIT=$GIT_COMMIT

ADD . /src
RUN (echo "${GIT_COMMIT}")
RUN (cd /src/cmd/wavelet; CGO_ENABLED=0 go build -ldflags "-X github.com/perlin-network/wavelet/sys.GitCommit=${GIT_COMMIT}")
RUN (cd /src/cmd/benchmark; CGO_ENABLED=0 go build)

FROM alpine:3.9

COPY --from=build /src/cmd/wavelet/ .
COPY --from=build /src/cmd/benchmark/benchmark .