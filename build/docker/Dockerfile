# Cross-compiles Boombrick for the TrimUI Brick (tg5040) with NextUI's toolchain.
# Digest of ghcr.io/loveretro/tg5040-toolchain:latest.
FROM ghcr.io/loveretro/tg5040-toolchain@sha256:f131c6af64029a8723d0ce8d3c2682642f5f091b04714f6beedda9bec18477ab

ARG GO_VERSION
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates wget pkg-config \
 && rm -rf /var/lib/apt/lists/* \
 && wget -qO- "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

# go-sdl2 finds SDL2 through pkg-config. Write a .pc file if the TrimUI SDK
# doesn't ship one.
RUN pc="${SYSROOT}/usr/lib/pkgconfig/sdl2.pc"; \
    if [ ! -f "$pc" ]; then \
      mkdir -p "$(dirname "$pc")"; \
      printf 'prefix=/usr\nlibdir=${prefix}/lib\nincludedir=${prefix}/include\n\nName: sdl2\nDescription: Simple DirectMedia Layer\nVersion: 2.0.0\nLibs: -L${libdir} -lSDL2\nCflags: -I${includedir}/SDL2 -D_REENTRANT\n' > "$pc"; \
    fi

ENV PATH=/usr/local/go/bin:${PATH} \
    CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
    CC=${CROSS_COMPILE}gcc CXX=${CROSS_COMPILE}g++ \
    CGO_CFLAGS="--sysroot=${SYSROOT} -O2" CGO_LDFLAGS="--sysroot=${SYSROOT}" \
    PKG_CONFIG_SYSROOT_DIR=${SYSROOT} PKG_CONFIG_LIBDIR=${SYSROOT}/usr/lib/pkgconfig

WORKDIR /src
