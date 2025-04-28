all: gotest atomfs

MAIN_VERSION ?= $(shell git describe --always --dirty || echo no-git)
ifeq ($(MAIN_VERSION),$(filter $(MAIN_VERSION), "", no-git))
$(error "Bad value for MAIN_VERSION: '$(MAIN_VERSION)'")
endif

ROOT := $(shell git rev-parse --show-toplevel)
GO_SRC_DIRS := $(shell find . -name "*.go" | xargs -n1 dirname | sort -u)
GO_SRC := $(shell find . -name "*.go")
VERSION_LDFLAGS=-X main.Version=$(MAIN_VERSION)
BATS = $(TOOLS_D)/bin/bats
BATS_VERSION := v1.10.0
PRE_EROFS_STACKER = $(TOOLS_D)/bin/pre-erofs-stacker
PRE_EROFS_STACKER_VERSION := v1.0.0
STACKER = $(TOOLS_D)/bin/stacker
STACKER_VERSION := v1.1.0-rc1
TOOLS_D := $(ROOT)/tools
GOCOVERDIR ?= $(ROOT)

export PATH := $(TOOLS_D)/bin:$(PATH)


.PHONY: gofmt
gofmt: .made-gofmt
.made-gofmt: $(GO_SRC)
	o=$$(gofmt -l -w $(GO_SRC_DIRS) 2>&1) && [ -z "$$o" ] || \
		{ echo "gofmt made changes: $$o" 1>&2; exit 1; }
	@touch $@

atomfs atomfs-cover: .made-gofmt $(GO_SRC)
	cd $(ROOT)/cmd/atomfs && go build $(BUILDCOVERFLAGS) -buildvcs=false -ldflags "$(VERSION_LDFLAGS)" -o $(ROOT)/bin/$@ ./...

atomfs-cover: BUILDCOVERFLAGS=-cover

gotest: $(GO_SRC)
	go test -coverprofile=unit-coverage.txt -ldflags "$(VERSION_LDFLAGS)"  ./...

$(PRE_EROFS_STACKER):
	mkdir -p $(TOOLS_D)/bin
	wget --progress=dot:giga https://github.com/project-stacker/stacker/releases/download/$(PRE_EROFS_STACKER_VERSION)/stacker --output-document $(TOOLS_D)/bin/pre-erofs-stacker
	chmod +x $(TOOLS_D)/bin/pre-erofs-stacker

$(STACKER):
	mkdir -p $(TOOLS_D)/bin
	wget --progress=dot:giga https://github.com/project-stacker/stacker/releases/download/$(STACKER_VERSION)/stacker --output-document $(TOOLS_D)/bin/stacker
	chmod +x $(TOOLS_D)/bin/stacker

$(BATS):
	mkdir -p $(TOOLS_D)/bin
	git clone -b $(BATS_VERSION) https://github.com/bats-core/bats-core.git
	cd bats-core; ./install.sh $(TOOLS_D)
	mkdir -p $(ROOT)/test/test_helper
	git clone --depth 1 https://github.com/bats-core/bats-support $(ROOT)/test/test_helper/bats-support
	git clone --depth 1 https://github.com/bats-core/bats-assert $(ROOT)/test/test_helper/bats-assert
	git clone --depth 1 https://github.com/bats-core/bats-file $(ROOT)/test/test_helper/bats-file

batstest: $(BATS) $(STACKER) $(PRE_EROFS_STACKER) atomfs-cover test/random.txt testimages
	cd $(ROOT)/test; sudo GOCOVERDIR=$(GOCOVERDIR) $(BATS) --tap --timing priv-*.bats
	cd $(ROOT)/test; GOCOVERDIR=$(GOCOVERDIR) $(BATS) --tap --timing unpriv-*.bats
	go tool covdata textfmt -i $(GOCOVERDIR) -o integ-coverage.txt

covreport: gotest batstest
	go tool cover -func=unit-coverage.txt
	go tool cover -func=integ-coverage.txt

testimages: /tmp/atomfs-test-oci/.copydone
	@echo "busybox image exists at /tmp/atomfs-test-oci already"

/tmp/atomfs-test-oci/.copydone:
	skopeo copy docker://public.ecr.aws/docker/library/busybox:stable oci:/tmp/atomfs-test-oci:busybox
	touch $@

test/random.txt:
	dd if=/dev/random of=/dev/stdout count=2048 | base64 > test/random.txt

.PHONY: test toolsclean
test: gotest batstest

toolsclean:
	rm -rf $(TOOLS_D)
	rm -rf $(ROOT)/test/test_helper
	rm -rf $(ROOT)/bats-core

clean:  toolsclean
	rm -rf $(ROOT)/bin
	rm -f .made-*
