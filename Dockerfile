FROM gcc:latest AS utils-build

RUN apt-get update && apt-get install -y flex libkeyutils-dev

WORKDIR /src
ARG DRBD_UTILS_SRC=https://pkg.linbit.com//downloads/drbd/utils/drbd-utils-9.33.0-rc.1.tar.gz
RUN curl -fsSL "$DRBD_UTILS_SRC" | tar -xzv --strip-components=1 \
    && LDFLAGS=-static CFLAGS=-static ./configure --without-84support --without-manual --without-drbdmon \
    && make \
    && make install

FROM --platform=$BUILDPLATFORM golang:1 AS go-build

WORKDIR /work
COPY go.mod go.sum /work/

RUN go mod download -x

COPY main.go main.go
COPY pkg/ pkg/

ARG VERSION=devel
ARG TARGETARCH
ARG TARGETOS
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -ldflags "-X github.com/piraeusdatastore/drbd-shutdown-guard/pkg/vars.Version=$VERSION" -o drbd-shutdown-guard main.go

FROM registry.access.redhat.com/ubi9/ubi:latest

COPY --link --from=utils-build /usr/local/sbin/drbdsetup /usr/local/sbin/drbdsetup
COPY --link --from=utils-build /usr/local/lib/drbd/tnf-drbd-fence.py /usr/local/lib/drbd/tnf-drbd-fence.py
COPY --link --from=go-build /work/drbd-shutdown-guard /usr/local/sbin/drbd-shutdown-guard

ENV DRBDSETUP_LOCATION=/usr/local/sbin/drbdsetup
ENV TNF_DRBD_FENCE_LOCATION=/usr/local/lib/drbd/tnf-drbd-fence.py
CMD ["/usr/local/sbin/drbd-shutdown-guard", "install"]
