go:
	go build


install:
	go install

vet:
	go vet

.PHONY: go install vet
