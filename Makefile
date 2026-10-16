GO      ?= go
VERSION ?= dev
PKG     := github.com/ujjwal313/boombrick
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)

.PHONY: build run test fonts fixtures clean

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/desktop/boombrick ./cmd/boombrick

run: build
	BOOMBRICK_DATA=dist/desktop/data ./dist/desktop/boombrick

test:
	$(GO) test ./...

fonts:
	scripts/fonts.sh

fixtures:
	scripts/fixtures.sh

clean:
	rm -rf dist

DEVICE_IMAGE := boombrick-tg5040
GO_VERSION   := $(shell $(GO) env GOVERSION 2>/dev/null | sed 's/^go//')
PAK_DIR      := dist/pak/Tools/tg5040/Boombrick.pak

.PHONY: device-image device pak

device-image:
	docker build --build-arg GO_VERSION=$(GO_VERSION) -t $(DEVICE_IMAGE) build/docker

device: device-image
	docker run --rm -v "$$(pwd)":/src \
		-v boombrick-gomod:/root/go/pkg/mod -v boombrick-gocache:/root/.cache/go-build \
		$(DEVICE_IMAGE) go build -trimpath -ldflags '$(LDFLAGS)' -o dist/tg5040/boombrick ./cmd/boombrick

pak: device
	rm -rf dist/pak
	mkdir -p $(PAK_DIR)
	cp dist/tg5040/boombrick build/pak/launch.sh LICENSE NOTICE $(PAK_DIR)/
	cp assets/fonts/OFL-*.txt $(PAK_DIR)/
	docker run --rm -v "$$(pwd)":/src $(DEVICE_IMAGE) sh -c \
		'mkdir -p $(PAK_DIR)/lib && cp -L $$SYSROOT/usr/lib/libopus.so.0 $$SYSROOT/usr/lib/libogg.so.0 $(PAK_DIR)/lib/'
	rm -f dist/Boombrick.pak.zip
	cd dist/pak && zip -qr ../Boombrick.pak.zip Tools
	@echo "dist/Boombrick.pak.zip"
