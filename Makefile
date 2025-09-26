.PHONY: build clean up run
build:
go build -o bin/sensor ./cmd/sensor
go build -o bin/gateway ./cmd/gateway
go build -o bin/appserver ./cmd/appserver


run-app:
./bin/appserver --config=configs/dev.yaml


clean:
rm -rf bin/
