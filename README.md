# odin-api-gen

A build-time Go tool that generates an [Odin](https://odin-lang.org) SDK
(client library) from an OpenAPI 3 spec. The Go program never ships — it reads
a spec and writes `.odin` files that you commit alongside your project:

```
openapi.json  →  [odin-api-gen (Go)]  →  sdk/<yourapi>/
                                          models.odin          structs + enum constants
                                          api_<tag>.odin       one proc per operation
                                          client.odin          hand-written runtime (copied in)
                                          transport_curl.odin  hand-written libcurl transport (copied in)
```

The reference target is the [PocketSmith API](https://github.com/pocketsmith/api)
(56 operations, 18 schemas): its spec (`openapi.json`) and generated SDK
(`sdk/pocketsmith/`) are committed here, and a golden-file test keeps them in
sync with the generator.

---

## Dependencies

| Dependency | Needed for | Notes |
|---|---|---|
| **Go** ≥ 1.26 | Running the generator | Build-time only; nothing Go ships in your program. Modules: `getkin/kin-openapi` (fetched by `go mod`). |
| **Odin compiler** | Building the generated SDK | Tested with `dev-2026-07`. The SDK uses only `core:` / `base:` packages (`encoding/json`, `strings`, `fmt`, `c`, `runtime`) — no third-party Odin dependencies. |
| **libcurl** (system) | The default HTTP transport (TLS included) | Linked as `system:curl`. **macOS**: ships with the OS, nothing to install. **Linux**: install the shared library (`libcurl4` on Debian/Ubuntu, `libcurl` on most others); typically already present. **Windows**: not assumed — the default transport returns an error and you must supply your own (see [Swapping the transport](#swapping-the-transport)). |

libcurl is only a dependency of `transport_curl.odin`. The generated API code
depends solely on the `Transport_Proc` interface in `client.odin`, so if you
replace the transport you have no libcurl dependency at all.

---

## Quick start (PocketSmith reference SDK)

```sh
make generate    # regenerate sdk/pocketsmith from openapi.json, then odin check
make test        # go test ./... (golden files + naming) && odin test tests
make example     # build examples/whoami against system libcurl
```

Live smoke test:

```sh
POCKETSMITH_DEVELOPER_KEY=... odin run examples/whoami
```

---

## Generating an SDK for your own API

### 1. Get an OpenAPI 3 spec

JSON or YAML, OpenAPI 3.0.x. Place it in the repo, e.g. `specs/myapi.json`.

### 2. Run the generator

```sh
go run . -spec specs/myapi.json -out sdk/myapi -package myapi
```

Flags:

| Flag | Meaning |
|---|---|
| `-spec` | Path to the OpenAPI 3 spec (JSON or YAML). |
| `-out` | Output directory for the generated Odin package. Created if missing; known files are overwritten. |
| `-package` | Odin package name for the generated files. |
| `-base-url` | Optional. Overrides the API base URL; defaults to the first `servers[].url` entry in the spec (the spec must have one otherwise). |

### 3. Expect it to fail loudly — and read the message

The resolver supports a bounded OpenAPI subset (see
[Supported OpenAPI subset](#supported-openapi-subset)) and **errors with the
exact spec location** on anything outside it, rather than emitting
plausible-but-wrong code. Typical messages:

```
odin-api-gen: GET /widgets/{id} param "filter": unsupported parameter type [array]
odin-api-gen: components.schemas.Widget.meta: additionalProperties is not supported
odin-api-gen: POST /widgets: operation has no operationId and no summary to derive a name from
```

Your options, in order of preference:

1. **Simplify the spec** — often the offending construct is incidental
   (e.g. an `allOf` wrapping a single `$ref`) and can be flattened.
2. **Extend the generator** — `internal/gen/resolve.go` is the walker,
   `internal/gen/templates/*.tmpl` the emission. Add support for what your
   spec actually uses, no more.
3. **Drop the endpoint** from the spec if you don't need it.

Spec requirements beyond the subset table:

- Every operation needs a **`summary`** (proc names are derived from it:
  "Get a transaction" → `get_transaction`) — or extend `resolve.go` to prefer
  `operationId` if your spec has good ones.
- Exactly **one 2xx response** per operation, with `application/json` content
  (or none, e.g. 204).
- Derived proc names must be unique; duplicates fail generation.

### 4. Adapt authentication in the runtime

The two runtime files are **hand-written** and live in
`internal/gen/runtime/`; the generator copies them into the output verbatim
(substituting only the package name and base URL). The stock `client.odin`
sends PocketSmith-style auth:

- `Client.developer_key` → `X-Developer-Key: <key>` header
- `Client.access_token` → `Authorization: Bearer <token>` header

Bearer tokens are standard OAuth 2 and work for many APIs as-is. If your API
uses a different scheme (a differently named API-key header, query-param keys,
basic auth), edit the `Client` struct and the header block in `_execute` in
`internal/gen/runtime/client.odin`, then regenerate. Keep the runtime generic:
it's shared by every SDK you generate from this repo.

### 5. Type-check and wire up regeneration

```sh
odin check sdk/myapi -no-entry-point
```

Add a Makefile target mirroring the `generate` target, and copy
`main_test.go`'s golden-file pattern for your output directory so CI catches
stale generated code. Commit the generated SDK: that's what makes generator
and spec changes reviewable as plain diffs.

### 6. Use the SDK from your Odin project

Copy or symlink the output directory into your project (it's a self-contained
Odin package) and import it:

```odin
import api "sdk/myapi"

client := api.client_from_developer_key(key)   // or client_from_oauth_token(tok)
client.base_url = "https://staging.example.com/v2"  // optional override

user, err := api.get_authorised_user(&client)
if err != nil {
	switch e in err {
	case api.Api_Error:       fmt.eprintln("HTTP", e.status, e.message)
	case api.Transport_Error: fmt.eprintln("network:", e.message)
	case api.Encode_Error, api.Decode_Error: fmt.eprintln("json:", err)
	}
	return
}
```

**Memory:** every generated proc takes a trailing
`allocator := context.allocator`; all returned data — decoded results and
error messages — is allocated from it. There are no `destroy_*` procs: pass an
arena or `context.temp_allocator` and free it wholesale when done. Internal
scratch (URLs, query strings, request bodies) uses `context.temp_allocator`.

**Optional query parameters** go in a per-operation `_Options` struct of
`Maybe(T)` fields; only the fields you set are sent:

```odin
txns, err := api.list_transactions_in_user(&client, user.id, api.List_Transactions_In_User_Options{
	start_date = "2024-01-01",
	search     = "cheese",
})
```

### Swapping the transport

`Client.transport` is a plain proc pointer:

```odin
Transport_Proc :: #type proc(data: rawptr, req: Http_Request, allocator: runtime.Allocator) -> (Http_Response, Maybe(Transport_Error))
```

Set it to use odin-http, a platform HTTP API, or a test mock (see
`tests/pocketsmith_test.odin` for a complete mock example — that's how the
whole SDK is tested without a network). Implementations must allocate the
response body with the passed `allocator`; request strings are only valid for
the duration of the call. This is also the escape hatch on Windows, where no
system libcurl is assumed.

---

## Design decisions

- **Type mapping**: `integer` → `i64`, `number` → `f64`, `string` → `string`,
  `boolean` → `bool`, arrays → slices. Fixed-width integers so 32-bit targets
  behave the same.
- **Optionality**: response-model fields are plain types (a missing key just
  leaves the zero value), except `nullable: true` fields which become
  `Maybe(T)`. Request-body fields not listed in `required` become
  `Maybe(T)` with `json:"...,omitempty"`, so partial updates only send what
  you set. (Consequence: you cannot send an explicit `null` to clear a field.)
- **Enums** are emitted as `string` plus named constants
  (`Transaction_Type_Debit :: "debit"`), because real-world values like
  `"no-interest"` and `"each weekday"` cannot be Odin enum variant names.
- **Only reachable schemas are emitted** — component schemas never referenced
  by an operation's request/response don't appear in `models.odin`.
- **Errors**: non-2xx responses are not modeled per-operation; they become
  `Api_Error{status, message}`, with `message` parsed from an
  `{"error": "..."}` body when present, else the raw body.

## Supported OpenAPI subset

| Supported | Not supported |
|---|---|
| `$ref` to component schemas (incl. recursion via arrays) | `allOf` / `anyOf` / discriminators |
| object / array / scalar schemas, `nullable` | free-form objects, `additionalProperties` |
| path + query params (default styles) | header/cookie params, exploded styles, array params |
| `application/json` bodies & responses | other content types, streaming (SSE etc.) |
| exactly one 2xx response per operation | multiple 2xx responses |
| `oneOf` at response top level → returned as `json.Value` | `oneOf` anywhere else |

## Repository layout

```
main.go                        CLI entry point
internal/gen/
  resolve.go                   OpenAPI → IR walker (loud errors on unsupported features)
  ir.go                        intermediate representation
  naming.go                    snake_case / Ada_Case / proc-name derivation
  emit.go                      template rendering + runtime copy
  templates/*.tmpl             models.odin and api_<tag>.odin templates
  runtime/*.odin               hand-written client + libcurl transport (source of truth)
openapi.json                   PocketSmith spec (reference target)
sdk/pocketsmith/               committed generated output (golden files)
tests/                         Odin mock-transport tests of the generated SDK
examples/whoami/               live smoke test (links system libcurl)
main_test.go                   golden-file test: regenerate + diff vs sdk/pocketsmith
```

## Testing

- `main_test.go` — golden-file test: regenerates into a temp dir and diffs
  against the committed `sdk/pocketsmith`, so generator changes show up as
  reviewable diffs of generated code. If a change is intentional, run
  `make generate` to refresh the committed output.
- `tests/` — Odin tests that drive the SDK through a mock transport and the
  real `core:encoding/json`: URL/query building and percent-encoding, auth
  headers, `omitempty` body marshaling, `Maybe`/`null` decoding, tolerance of
  unknown response fields, and error mapping. `odin test tests` also proves
  the libcurl bindings compile and link.
- `internal/gen/naming_test.go` — name-derivation unit tests.

## Regeneration workflow

1. Update the spec (or the generator).
2. `make generate` — regenerates and type-checks.
3. `make test` — golden diff + Odin behavior tests.
4. Review the diff of the generated SDK like any other code change.
