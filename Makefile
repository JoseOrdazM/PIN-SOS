.PHONY: build run clean

build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o pinsos .

run: build
	./pinsos

clean:
	rm -f pinsos
	rm -rf uploads/*
