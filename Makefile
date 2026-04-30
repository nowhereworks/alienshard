.PHONY: serve docker-build build install

serve:
	alienshard serve

docker-build:
	docker build . -t alienshard:latest

build:
	go build .

install: build
	go install .
