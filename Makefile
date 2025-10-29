.PHONY: help build install test clean run generate-test

help:
	@echo "Ahoy Language Compiler"
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build         - Build ahoy-bin"
	@echo "  test          - Run tests"
	@echo "  clean         - Clean artifacts"
	@echo "  generate-test - Generate test case from FILE=path/to/file.ahoy"

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

generate-test:
	@if [ -z "$(FILE)" ]; then \
		echo "Usage: make generate-test FILE=test/input/your_file.ahoy"; \
		exit 1; \
	fi
	@cd source && go run generate_tests.go tokenizer.go parser.go codegen.go formatter.go ../$(FILE)
