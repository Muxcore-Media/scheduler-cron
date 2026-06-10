.PHONY: build test lint clean

build:
	go build -o scheduler-cron ./cmd/module

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run

clean:
	rm -f scheduler-cron
	rm -f cmd/module/module
