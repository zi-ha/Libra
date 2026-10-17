# Libra 常用构建/部署命令
VERSION ?= 1.0.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help tidy fmt vet test build run linux-arm64 linux-amd64 docker samples clean

help:
	@echo "make build         编译当前平台"
	@echo "make run           本地起服务（数据目录 ./data）"
	@echo "make fmt           格式化 Go 代码"
	@echo "make vet           静态检查"
	@echo "make test          运行 Go 测试"
	@echo "make linux-amd64   交叉编译到 Linux x86_64（群晖/威联通 x86 机型）"
	@echo "make linux-arm64   交叉编译到 Linux ARM64（树莓派 / M 系列 NAS）"
	@echo "make docker        构建 Docker 镜像"
	@echo "make samples       生成联调样本（EPUB/PDF）"
	@echo "make clean         清理本地产物"

tidy:
	go mod tidy

fmt:
	gofmt -w main.go internal

vet:
	go vet ./...

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o libra .

linux-amd64:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/libra-linux-amd64 .

linux-arm64:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/libra-linux-arm64 .

docker:
	docker build --build-arg VERSION=$(VERSION) -t libra:$(VERSION) .

samples:
	python3 scripts/gen_samples.py

run: build
	LIBRA_DATA_DIR=./data ./libra

clean:
	rm -rf dist libra libra.exe
