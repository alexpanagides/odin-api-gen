# Code generator

> Is there oapi-codegen for Odin?

No — nothing like it exists. Odin isn't in OpenAPI Generator's supported list either (which has ~140 targets including Nim, Crystal, Eiffel, and GDScript — Odin just hasn't been contributed). There's no standalone Odin equivalent I can find.

Realistic paths, roughly in order of effort:

**Write your own generator in Go.** Probably your best option given your background. Parse the spec with `pb33f/libopenapi` or `getkin/kin-openapi`, emit Odin with `text/template`. Odin maps cleanly enough — structs with `json:"..."` tags work with `core:encoding/json` marshal/unmarshal, and optional fields become `Maybe(T)`. You're mostly writing a schema→struct walker plus a thin request-builder template. A few hundred lines gets you the 80% case.

**Custom templates on openapi-generator.** You can scaffold a new generator with `openapi-generator-cli meta`, but that means a Java class plus Mustache templates. More ceremony than value unless you want it upstreamable.

**Generate C and bind via FFI.** The `c` generator exists, but you'd be fighting the generated C's memory model and then writing bindings anyway. Worse than option 1.

One thing to plan for: Odin's core has `net` (sockets) but no HTTP client. You'd generate against laytan/odin-http, which has a client package, though the author notes it's still heavily in development and pushes breaking API changes. Worth pinning a commit if you build a generator on top of it.


> I’m thinking of using Fable 5 to create an Odin OpenAI generator. Thoughts?

Good fit for the task, with one framing that matters a lot: have the model write the *generator*, not the generated code. A one-shot "here's the OpenAI spec, emit an Odin SDK" gives you thousands of lines nobody can regenerate or diff when the spec updates. You want a Go program you can re-run.
Things I'd plan around:
* **Odin is thin in training data.** Any model will confidently invent core:encoding/json signatures, allocator conventions, and Maybe(T) semantics. Don't rely on recall — paste the actual source of core:encoding/json and the odin-http client package into context, and put odin build in the loop so hallucinated APIs die on the first compile.
* **Hand-write the target first.** Pick 2–3 endpoints, write the Odin client by hand the way you'd want it to look, then point the model at that as the golden output and work backwards to templates. Much better signal than describing the style in prose.
* **Bound the OpenAPI subset.** oneOf/allOf/discriminators, nullable-vs-optional, additionalProperties, and param serialization styles are where these generators go to die. Decide up front you only support what your actual target specs use, and make the generator error loudly on the rest rather than emitting something plausible.
* **Golden-file tests from day one.** Commit generated output, regenerate in CI, diff. That's what makes the model's changes reviewable instead of a wall of churn.

⠀If the eventual goal is an Odin OpenAI client specifically — OpenAI does publish an OpenAPI spec, so that's a decent forcing function: a real, large, messy spec as your first test case rather than a toy petstore. Just be aware streaming (SSE for chat completions) won't come out of the generator and you'll be hand-writing that layer regardless.

> So Go takes the spec then generates Odin language?

Yes. The Go program is a build-time tool that never ships — it reads the OpenAPI JSON/YAML and writes `.odin` files. Nothing Go touches your Luckfox or any runtime.

```
openai-spec.yaml  →  [gen (Go binary)]  →  client/
                                            models.odin
                                            chat.odin
                                            embeddings.odin
```

The Go choice is just because that's where the good OpenAPI parsers live (`libopenapi`, `kin-openapi`) — they handle `$ref` resolution, which is the tedious part. You could write it in Odin itself for purity, but you'd be building spec parsing from scratch first.

Practically: `gen` walks the resolved schema tree, maps each type to an Odin struct with json tags, then walks paths to emit one proc per operation. `text/template` for the emission. You commit both the generator and its output, and re-run it when OpenAI bumps the spec.

One nicety of this split: the generator is plain Go with no dependency on Odin at all, so you can unit-test it fast, and `odin build` on the output is your integration test.













#Personal/odin