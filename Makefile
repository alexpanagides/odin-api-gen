SDK_DIR = sdk/pocketsmith

.PHONY: generate check test test-go test-odin example clean

# Regenerate the Odin SDK from openapi.json and type-check it.
generate:
	go run . -spec openapi.json -out $(SDK_DIR) -package pocketsmith
	odin check $(SDK_DIR) -no-entry-point

check:
	odin check $(SDK_DIR) -no-entry-point

test: test-go test-odin

# Golden-file test: fails if $(SDK_DIR) is stale relative to the generator.
test-go:
	go test ./...

# Mock-transport tests of the generated SDK against real core:encoding/json.
test-odin:
	odin test tests

example:
	odin build examples/whoami -out:whoami

clean:
	rm -f whoami

claude:
	CLAUDE_CONFIG_DIR=/Users/alexispanagides/GolandProjects/quicken_cvrt/.claude-project claude
