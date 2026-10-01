# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A build-time Go tool that turns an OpenAPI 3 spec into an Odin SDK package. The Go code never ships; its output (`.odin` files) is committed. The reference target is the PocketSmith API: `openapi.json` → `sdk/pocketsmith/`.

## Commands

```sh
make generate    # go run . -spec openapi.json -out sdk/pocketsmith -package pocketsmith, then odin check
make check       # odin check sdk/pocketsmith -no-entry-point
make test        # test-go + test-odin
make test-go     # go test ./...  (golden-file test + naming tests)
make test-odin   # odin test tests  (mock-transport tests of the generated SDK)
make example     # odin build examples/whoami -out:whoami

go test ./internal/gen -run TestSnakeCase              # single Go test
odin test tests -define:ODIN_TEST_NAMES=sdk_tests.test_oauth_bearer_header   # single Odin test
odin check sdk/pocketsmith -no-entry-point -target:windows_amd64  # cross-target type-check
POCKETSMITH_DEVELOPER_KEY=... odin run examples/whoami            # live smoke test
```

**Any change to the generator, templates, or runtime requires `make generate`** and committing the regenerated `sdk/pocketsmith/`. `main_test.go` (`TestGoldenSDK`) regenerates into a temp dir and fails on any byte difference with the committed SDK, including files that were added or removed.

## Pipeline

`main.go` loads and validates the spec with kin-openapi (example validation is disabled because the PocketSmith spec has invalid examples), then runs two stages in `internal/gen`:

1. **`Resolve`** (`resolve.go`): walks the spec and produces the IR in `ir.go`, which contains no OpenAPI types. All naming, type mapping, optionality, enum collection, and support checks happen here. It intentionally supports only a bounded subset of OpenAPI. On anything else it **fails with the spec location** (`GET /x param "y": unsupported …`) instead of emitting a best guess. Keep that behavior when extending it, and add only what a real spec needs.
2. **`Emit`** (`emit.go`): renders `templates/models.odin.tmpl` (one file) and `templates/api.odin.tmpl` (one `api_<tag>.odin` per tag; the options structs live with their operations, not in models). It then copies **every file in `internal/gen/runtime/`** into the output unchanged. The only substitutions are the first occurrence of `package PACKAGE_NAME` and of `BASE_URL_PLACEHOLDER`.

Naming rules are in `naming.go`: proc names come from the operation `summary` ("Get a transaction" → `get_transaction`), types use Ada_Case, and fields use snake_case with keyword escaping.

## Hand-written runtime (`internal/gen/runtime/`)

`internal/gen/runtime/` holds the source of truth. Never edit `sdk/pocketsmith/client.odin` or `transport_curl.odin` directly, because they're regenerated copies.

- `client.odin`: `Client`, the `Error` union (`Api_Error | Transport_Error | Encode_Error | Decode_Error`), `_execute` (auth headers live here), JSON `_encode`/`_decode`, and the `_Query_Builder`.
- `transport_curl.odin`: the default `Transport_Proc`, built on `vendor:curl`. It's the only file that imports curl. Generated code depends only on the `Transport_Proc` interface, which is how `tests/` swap in a mock transport.

Couplings between files that are easy to break:
- Generated code calls runtime procs by name. `api.odin.tmpl` emits `_qb_add_<OdinType>` / `_qb_maybe_<OdinType>` (via the `qbAdd`/`qbMaybe` funcs in `emit.go`), so each scalar param type (`string`, `i64`, `f64`, `bool`) needs matching procs in `client.odin`.
- `reservedNames` in `resolve.go` must list every public type name the runtime defines, so spec models can't collide with them. Update it when you add a public runtime type.
- The runtime is shared by every SDK generated from this repo, so keep it API-agnostic.

## Generated-code conventions

- Every generated proc takes a trailing `allocator := context.allocator`, and all returned data comes from it. There are no `destroy_*` procs; callers free arenas wholesale. Internal scratch work uses `context.temp_allocator`.
- Response fields use plain types. `nullable` fields become `Maybe(T)`. Non-required request-body fields become `Maybe(T)` with `omitempty`.
- Optional query params go in a per-operation `<Proc>_Options` struct. Required ones are positional args.
- Enums become `string` plus named constants, because real values like `"no-interest"` aren't valid Odin identifiers.
- A top-level `oneOf` response returns `json.Value`. Non-2xx responses become `Api_Error`.

## Dependencies

- Go ≥ 1.26 (kin-openapi) for the generator.
- An Odin compiler for the SDK. The SDK uses only `core:`/`base:` packages plus `vendor:curl`. On Linux, `vendor:curl` links `curl`, `z`, `mbedtls`, `mbedx509`, `mbedcrypto`. On Windows it uses the static lib bundled with Odin.

## Repo hygiene

`make claude` runs Claude Code with `CLAUDE_CONFIG_DIR` set to the repo root, so Claude Code's config and session files show up untracked at the top level (`.claude.json`, `projects/`, `sessions/`, `history.jsonl`, …). Never `git add` them, and never commit the `whoami` binary.
