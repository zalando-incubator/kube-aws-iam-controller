.PHONY: clean test check build.local build.linux build.osx build.docker build.push

BINARY        ?= kube-aws-iam-controller
VERSION       ?= $(shell git describe --tags --always --dirty)
IMAGE         ?= registry-write.opensource.zalan.do/teapot/$(BINARY)
TAG           ?= $(VERSION)
SOURCES       = $(shell find . -name '*.go')
GENERATED     = pkg/client pkg/apis/zalando.org/v1/zz_generated.deepcopy.go
DOCKERFILE    ?= Dockerfile
GOPKGS        = $(shell go list ./...)
BUILD_FLAGS   ?= -v
LDFLAGS       ?= -X main.version=$(VERSION) -w -s

default: build.local

clean:
	rm -rf build
	rm -rf $(GENERATED)

test: go.mod $(GENERATED)
	go test -v $(GOPKGS)

check: go.mod $(GENERATED)
	golint $(GOPKGS)
	go vet -v $(GOPKGS)

$(GENERATED):
	./hack/update-codegen.sh

build.local: build/$(BINARY)

build/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	CGO_ENABLED=0 go build -o build/$(BINARY) $(BUILD_FLAGS) -ldflags "$(LDFLAGS)" .

build.linux: build/linux/$(BINARY)
build.linux.amd64: build/linux/amd64/$(BINARY)
build.linux.arm64: build/linux/arm64/$(BINARY)

build/linux/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	GOOS=linux CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build/linux/amd64/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build/linux/arm64/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/linux/$(BINARY) -ldflags "$(LDFLAGS)" .

build.osx: build/osx/$(BINARY)
build.osx.amd64: build/osx/amd64/$(BINARY)
build.osx.arm64: build/osx/arm64/$(BINARY)

build/osx/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	GOOS=darwin CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" .

build/osx/amd64/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" .

build/osx/arm64/$(BINARY): go.mod $(GENERATED) $(SOURCES)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/osx/$(BINARY) -ldflags "$(LDFLAGS)" .

build.docker: build.linux
	docker build --rm -t "$(IMAGE):$(TAG)" -f $(DOCKERFILE) .

build.push: build.docker
	docker push "$(IMAGE):$(TAG)"

build.push.amd64: build.linux.amd64
	docker buildx build --rm -t "$(IMAGE):$(TAG)-amd64" -f $(DOCKERFILE) --push --platform linux/amd64 .

build.push.arm64: build.linux.arm64
	docker buildx build --rm -t "$(IMAGE):$(TAG)-arm64" -f $(DOCKERFILE) --push --platform linux/arm64 .

build.push.manifest: build.push.amd64 build.push.arm64
	docker manifest create "$(IMAGE):$(TAG)" "$(IMAGE):$(TAG)-amd64" "$(IMAGE):$(TAG)-arm64"
	docker manifest push "$(IMAGE):$(TAG)"
