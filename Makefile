build-master:
	go build -o bin/master ./cmd/master/

run-master: build-master
	./bin/master

build-farmer:
	go build -o bin/farmer ./cmd/farmer/

run-farmer: build-farmer
	./bin/farmer

usage:
	@echo "Usage:"
	@echo "  make build-master   # Build the master binary"
	@echo "  make run-master     # Build and run the master"
	@echo "  make build-farmer   # Build the farmer binary"
	@echo "  make run-farmer     # Build and run the farmer"
	@echo "  make usage          # Show this usage message"