.PHONY: web build test race clean

web:
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=1 go build -buildmode=c-shared -o plugins/cpa-guard.dll .

test:
	go test ./...

race:
	go test -race ./...

clean:
	rm -rf plugins web/dist web/node_modules
