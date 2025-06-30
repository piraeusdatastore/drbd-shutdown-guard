# === Stage 1: Build DRBD utilities ===
FROM --platform=$TARGETPLATFORM gcc:13 as utils-builder

# Install only necessary tools in minimal environment
# - flex: needed for parsing in DRBD utils build
# - libkeyutils-dev: required by DRBD
# - curl, tar: to fetch and extract source
# - make: to compile tools
RUN apt-get update && apt-get install -y \
    flex \
    libkeyutils-dev \
    curl \
    tar \
    make \
 && apt-get clean

ARG DRBD_UTILS_VERSION=9.27.0
RUN curl -fsSL "https://pkg.linbit.com/downloads/drbd/utils/drbd-utils-$DRBD_UTILS_VERSION.tar.gz" | tar -xzv \
    && cd "drbd-utils-$DRBD_UTILS_VERSION" \
    && LDFLAGS=-static CFLAGS=-static ./configure --without-83support --without-84support --without-manual --without-drbdmon \
    && make tools \
    && mv user/v9/drbdsetup /drbdsetup

# === Stage 2: Build Go binary ===
FROM --platform=$BUILDPLATFORM golang:1 as go-builder

WORKDIR /work
COPY go.mod go.sum /work/
RUN go mod download -x

COPY main.go main.go
COPY pkg/ pkg/

ARG VERSION=devel
ARG TARGETARCH
ARG TARGETOS
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -a -ldflags "-X github.com/piraeusdatastore/drbd-shutdown-guard/pkg/vars.Version=$VERSION" -o drbd-shutdown-guard main.go

# === Stage 3: Final minimal runtime image ===
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Copy only the built binaries from previous stages
COPY --from=utils-builder /drbdsetup /usr/local/sbin/drbdsetup
COPY --from=go-builder /work/drbd-shutdown-guard /usr/local/sbin/drbd-shutdown-guard

# Define the binary to run
ENV DRBDSETUP_LOCATION=/usr/local/sbin/drbdsetup
CMD ["/usr/local/sbin/drbd-shutdown-guard", "install"]
