FROM golang:1.12.1-alpine as build

RUN echo "@testing http://nl.alpinelinux.org/alpine/edge/testing" >>/etc/apk/repositories
RUN apk add --no-cache git coreutils curl cmake gcc g++ build-base perl bash linux-headers snappy-dev zlib-dev bzip2-dev lz4-dev zstd-dev

RUN (cd /tmp; git clone https://github.com/gflags/gflags.git) \
 && (cd /tmp/gflags; mkdir build; cd build; cmake -DBUILD_SHARED_LIBS=1 -DGFLAGS_INSTALL_SHARED_LIBS=1 ..; make -j$(nproc) install) \
 && rm -R /tmp/gflags/

RUN (cd /tmp; curl -L https://github.com/facebook/rocksdb/archive/v5.18.3.tar.gz | tar xzf -) \
 && (cd /tmp/rocksdb-5.18.3; make -j$(nproc) install-shared; exit 0) \
 && (cd /tmp/rocksdb-5.18.3; cp -r include /usr/local/rocksdb/) \
 && (cd /tmp/rocksdb-5.18.3; cp -r include/* /usr/include/) \
 && rm -R /tmp/rocksdb-5.18.3

RUN mkdir /src
ADD . /src
RUN (cd /src/cmd/wavelet; go build)



FROM alpine:3.9

RUN echo "@testing http://nl.alpinelinux.org/alpine/edge/testing" >>/etc/apk/repositories
RUN apk add --no-cache snappy-dev zlib-dev bzip2-dev lz4-dev zstd-dev

RUN mkdir /exec
COPY --from=build /usr/local/lib/librocksdb* /usr/local/lib/
COPY --from=build /usr/local/lib/libgflags* /usr/local/lib/
COPY --from=build /src/cmd/wavelet/wavelet /exec/wavelet
ENTRYPOINT ["/exec/wavelet"]