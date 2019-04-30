FROM golang:1.12.1 as build

RUN mkdir /src
COPY go.mod /src/go.mod
COPY go.sum /src/go.sum
RUN (cd /src; go mod download)

ADD . /src
RUN (cd /src/cmd/wavelet; go build)

FROM alpine:3.9

RUN mkdir /exec
COPY --from=build /src/cmd/wavelet/ .
ENTRYPOINT ["./wavelet"]