build-master:
	go build -o bin/master ./cmd/master/

run-master: build-master
	./bin/master

build-farmer:
	go build -o bin/farmer ./cmd/farmer/

run-farmer: build-farmer
	./bin/farmer
	