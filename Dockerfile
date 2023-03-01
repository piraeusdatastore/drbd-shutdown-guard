FROM gcc:latest as utils-builder

RUN apt-get update && apt-get install -y flex

ARG DRBD_UTILS_VERSION=9.23.0
RUN curl -fsSL "https://pkg.linbit.com/downloads/drbd/utils/drbd-utils-$DRBD_UTILS_VERSION.tar.gz" | tar -xzv \
    && cd "drbd-utils-$DRBD_UTILS_VERSION" \
    && LDFLAGS=-static CFLAGS=-static ./configure --without-83support --without-84support --without-manual --without-drbdmon \
    && make tools \
    && mv user/v9/drbdsetup /drbdsetup

FROM golang:1 as go-builder

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

COPY --from=utils-builder /drbdsetup /usr/local/sbin/drbdsetup
COPY --from=go-builder /work/drbd-shutdown-guard /usr/local/sbin/drbd-shutdown-guard

ENV DRBDSETUP_LOCATION=/usr/local/sbin/drbdsetup
CMD ["/usr/local/sbin/drbd-shutdown-guard", "install"]
