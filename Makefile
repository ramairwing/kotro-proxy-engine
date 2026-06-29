.PHONY: build test bench mock proxy run clean

build: proxy mock

proxy:
	go build -o bin/kortolabs-proxy ./cmd/proxy

mock:
	go build -o bin/mock-upstream ./cmd/mockupstream

test:
	go test ./...

bench:
	go test -bench=. -benchmem ./...

run: build
	@echo "Start mock upstream: bin/mock-upstream"
	@echo "Start proxy: KORTO_UPSTREAM_URL=http://127.0.0.1:9000 bin/kortolabs-proxy"

clean:
	rm -rf bin/ kortolabs-cache.db
