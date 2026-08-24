BIN := bin/herdr-golden-ratio

.PHONY: build test vet e2e clean

build:
	go build -o $(BIN) .

test:
	go test ./...

vet:
	go vet ./...

# End-to-end tests against a throwaway server; never touches a real session.
e2e: build
	@test/isolated-server.sh start >/dev/null
	@HERDR_SOCKET_PATH="$$(test/isolated-server.sh socket)" \
	 XDG_CONFIG_HOME="$$(dirname "$$(dirname "$$(test/isolated-server.sh socket)")")" \
	 env -u HERDR_PANE_ID -u HERDR_TAB_ID -u HERDR_WORKSPACE_ID test/e2e.sh; \
	 status=$$?; test/isolated-server.sh stop >/dev/null; exit $$status

clean:
	rm -rf bin
