FROM golang:1.12.5-alpine as build
RUN apk add --no-cache git

RUN mkdir /src
COPY go.mod /src/go.mod
COPY go.sum /src/go.sum
RUN (cd /src; go mod download)

ADD . /src
RUN (cd /src/cmd/wavelet; go build)
RUN (cd /src/cmd/benchmark; go build)

FROM alpine:3.9

COPY --from=build /src/cmd/wavelet/ .
COPY --from=build /src/cmd/benchmark/benchmark .