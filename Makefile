GO      ?= go
VERSION ?= dev
PKG     := github.com/ujjwal313/boombrick
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)

# QuickJS-NG, yt-dlp's JavaScript runtime: the project's static builds,
# checked against these sums.
QJS_VERSION     := v0.17.0
QJS_URL         := https://github.com/quickjs-ng/quickjs/releases/download/$(QJS_VERSION)
QJS_SHA_aarch64 := 3372133484edf50a69f3c67903af41206d22a061e930e3cfb63269272ef56d2e
QJS_SHA_x86_64  := 0bfc02511a9f549c28b53880d988fc7cd5d361e90c5e8afdfcd7dc6774ceace5

.PHONY: build run test fonts fixtures clean

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o dist/desktop/boombrick ./cmd/boombrick

build: dist/desktop/bin/qjs

dist/desktop/bin/qjs: dist/qjs/qjs-linux-$(shell uname -m)
	mkdir -p $(@D)
	cp $< $@

dist/qjs/qjs-linux-%:
	mkdir -p $(@D)
	curl -fsSLo $@.part $(QJS_URL)/qjs-linux-$*
	echo "$(QJS_SHA_$*)  $@.part" | sha256sum -c --quiet
	chmod +x $@.part
	mv $@.part $@

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

pak: device dist/qjs/qjs-linux-aarch64
	rm -rf dist/pak
	mkdir -p $(PAK_DIR)
	cp dist/tg5040/boombrick build/pak/launch.sh LICENSE NOTICE $(PAK_DIR)/
	mkdir -p $(PAK_DIR)/bin
	cp dist/qjs/qjs-linux-aarch64 $(PAK_DIR)/bin/qjs
	cp assets/fonts/OFL-*.txt $(PAK_DIR)/
	docker run --rm -v "$$(pwd)":/src $(DEVICE_IMAGE) sh -c \
		'mkdir -p $(PAK_DIR)/lib && cp -L $$SYSROOT/usr/lib/libopus.so.0 $$SYSROOT/usr/lib/libogg.so.0 $(PAK_DIR)/lib/'
	rm -f dist/Boombrick.pak.zip
	cd dist/pak && zip -qr ../Boombrick.pak.zip Tools
	@echo "dist/Boombrick.pak.zip"
