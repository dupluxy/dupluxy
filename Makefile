
GO ?= go
SHASUM ?= shasum -a 256
HAS_GO := $(shell hash $(GO) >/dev/null 2>&1 && echo yes)
COMMA := ,

ifeq ($(HAS_GO), yes)
  GOPATH ?= $(shell $(GO) env GOPATH)
  export PATH := $(GOPATH)/bin:$(PATH)
endif

OUTDIR ?= $(CURDIR)/out

define do_go_build
	@case "$1" in \
		x64) GOARCH=amd64 ;; \
		i386) GOARCH=386 ;; \
		*) GOARCH="$1" ;; \
	esac; \
	case "$2" in \
		win) GOOS=windows ;; \
		osx) GOOS=darwin ;; \
		*) GOOS="$2" ;; \
	esac; \
	export GOOS GOARCH; \
	echo "Building $$GOOS/$$GOARCH..." ; \
	$(GO) build -o $@ -ldflags "-s -X main.Version=$$(git describe --tags --always --dirty) -X main.GitCommit=$$(git rev-parse --short=7 HEAD)" ./dupluxy
endef

$(OUTDIR)/dupluxy_%:
	@mkdir -p $(OUTDIR)
	$(let os arch, $(subst _, ,$*), $(call do_go_build,$(basename $(arch)),$(os)))
#	$(SHASUM) $$@ > $$@.sha256

BUILDS := linux/x64 linux/arm64 linux/arm linux/i386 osx/x64 osx/arm64 win/x64 win/i386 freebsd/x64 freebsd/arm64 freebsd/i386

build-all: $(foreach cfg,$(BUILDS), $(let os arch,$(subst /, ,$(cfg)), $(OUTDIR)/dupluxy_$(os)_$(arch)$(if $(findstring win,$(os)),.exe)))

clean:
	rm -rf $(OUTDIR)

.PHONY: build-all clean
