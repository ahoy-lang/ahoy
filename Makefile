.PHONY: help build install test clean run

help:
@echo "Ahoy Language Compiler"
@echo "Usage: make [target]"
@echo ""
@echo "Targets:"
@echo "  build   - Build ahoy-bin"
@echo "  test    - Run tests"
@echo "  clean   - Clean artifacts"

build:
cd source && go build -o ../ahoy-bin .

install: build
mkdir -p ~/bin
cp ahoy-bin ~/bin/ahoy
chmod +x ~/bin/ahoy

test:
cd source && go test -v

clean:
rm -f ahoy-bin
rm -rf output/*

run:
cd source && go run . -f ../$(FILE) -r
