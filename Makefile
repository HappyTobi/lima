GO ?= go

TAR ?= tar

PACKAGE := github.com/AkihiroSuda/lima

VERSION=$(shell git describe --match 'v[0-9]*' --dirty='.m' --always --tags)
VERSION_TRIMMED := $(VERSION:v%=%)

GO_BUILD := CGO_ENABLED=0 $(GO) build -ldflags="-s -w -X $(PACKAGE)/pkg/version.Version=$(VERSION)"

.PHONY: all
all: binaries

.PHONY: _output/bin/macvirt
_output/bin/macvirt: 
	mkdir -p _output/bin/
	cd tools/macvirt && swift build -c release --disable-sandbox
	cp tools/macvirt/.build/release/macvirt _output/bin/macvirt
	codesign -s - --entitlements tools/macvirt/macvirt.entitlements _output/bin/macvirt
	chmod +x _output/bin/macvirt

.PHONY: binaries
binaries: \
	_output/bin/lima \
	_output/bin/limactl \
	_output/bin/nerdctl.lima \
	_output/share/lima/lima-guestagent.Linux-x86_64 \
	_output/share/lima/lima-guestagent.Linux-aarch64 \
	_output/bin/macvirt

.PHONY: _output/bin/lima
_output/bin/lima:
	mkdir -p _output/bin
	cp -a ./cmd/lima $@

.PHONY: _output/bin/nerdctl.lima
_output/bin/nerdctl.lima:
	mkdir -p _output/bin
	cp -a ./cmd/nerdctl.lima $@

.PHONY: _output/bin/limactl
_output/bin/limactl:
	$(GO_BUILD) -o $@ ./cmd/limactl

.PHONY: _output/share/lima/lima-guestagent.Linux-x86_64
_output/share/lima/lima-guestagent.Linux-x86_64:
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $@ ./cmd/lima-guestagent
	chmod 644 $@

.PHONY: _output/share/lima/lima-guestagent.Linux-aarch64
_output/share/lima/lima-guestagent.Linux-aarch64:
	GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $@ ./cmd/lima-guestagent
	chmod 644 $@

.PHONY: install
install:
	cp -av _output/* /usr/local/
	if [ ! -e /usr/local/bin/nerdctl ]; then ln -sf nerdctl.lima /usr/local/bin/nerdctl; fi

.PHONY: clean
clean:
	rm -rf _output

.PHONY: artifacts
artifacts:
	mkdir -p _artifacts
	GOOS=darwin GOARCH=amd64 make clean binaries
	$(TAR) -C _output/ -czvf _artifacts/lima-$(VERSION_TRIMMED)-Darwin-x86_64.tar.gz ./
	GOOS=darwin GOARCH=arm64 make clean binaries
	$(TAR) -C _output -czvf _artifacts/lima-$(VERSION_TRIMMED)-Darwin-arm64.tar.gz ./
