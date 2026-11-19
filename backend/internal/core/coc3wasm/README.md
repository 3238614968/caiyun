# COC3 wheel in Go

`Generate` runs the unchanged official `wheel_bg.wasm` with wazero v1.12.0.
It calls `wheel_roto_simple("{}")` from official wheel 0.0.8 and retains the wire format
`[aurora, content]`. There is no JavaScript interpreter or subprocess in this
path, and it supports `CGO_ENABLED=0` builds.

Each generation has its own instance, memory and wasm-bindgen handle heap.
Compiled code is cached across calls. Guest memory is limited to 16 MiB; guest
loops can be interrupted by context cancellation. The task applies a 30-second
deadline. Invalid modules, ABI changes, random-source failures and oversized
headers return errors instead of an empty token.

The host provides an isolated page descriptor for the simple entry: the same
mobile User-Agent and canvas URL as the HTTP client, with no DOM or Performance
object. It also provides the clock, `crypto/rand`, string/reference operations
and the wheel stack descriptor.
It never exposes host globals, `process.env`, filesystem or networking APIs.
Unsupported host calls fail explicitly. The official encryption module remains
unchanged; this does not manufacture account eligibility or device attestation.

## Offline compatibility vectors

`testdata/reference.json` contains outputs of the official JavaScript wrapper
using the same isolated environment, fixed clocks and deterministic entropy.
Go tests compare both strings byte for byte and also verify fresh concurrent
instances, cancellation, memory limits, entropy failure and operation with an
empty `PATH`. The tests and production do not need Node.js.

To regenerate vectors after an upstream asset update, developers can run:

```text
node backend/internal/core/coc3wasm/testdata/generate-reference.mjs
```

This development-only script uses no real account credentials or host process
environment. Upstream runtime documentation: https://wazero.io/docs/.
