# COC3 wheel resources

`wheel_bg.wasm` is the unchanged official COC3 canvas wheel 0.0.8 resource,
retrieved on 2026-11-12 from the URLs loaded by the official canvas page:

- https://res.coc.10086.cn/res/cdn/coc1-online/fixedPath/coc3/WASM/wheel-0.0.8.js
- https://res.coc.10086.cn/res/cdn/coc1-online/fixedPath/coc3/WASM/wheel_bg-0.0.8.wasm
- WASM SHA-256: `4f6f69cf9a46a8a2c3fb23ed4c31b08a9da5b2afc1bc2149ee7364c77f0570a8`.

It is embedded in the backend and
executed by the pure Go wazero runtime in `internal/core/coc3wasm`.

The generation path does not execute JavaScript or start a process. It requires
no Node.js, browser, CGO or writable runtime directory. Each call owns a separate
guest instance with bounded memory and a cancellation deadline. Host globals,
process environment variables and files are unavailable.

The production path now runs the official `g_dt_with_custom_url` preflight and
`roto_simple("{}")` on the same instance. Its transport permits exactly one POST
to `https://dev.coc.10086.cn/coc3/gr/risk-ability/wasm/gettoken`, using the current
COC business session. HTTP 200, business code 0, `success=true` and a nonempty
device receipt are independently required before the guest receives the result.
Cloud Basic/JWT/device headers and shared cookies are excluded. Browser features
that are absent in Go remain absent. The default offline `Generate` entry still
has no network capability.

Native ConstID lookup and its one requested detection stage are implemented in
`api/caiyun_coc3_risk.go`. The latter describes only the restricted host: absent
rendering/source properties remain unknown and the SDK's environment mismatch
finding is retained. The external risk service receives no account credentials.
Only a real successful receipt yields a risk token; repeated challenges stop.

Native BSFIT packet generation is in `api/caiyun_coc3_fingerprint*.go`. It uses
Fingerprint2-compatible hashing and the upstream wire cipher, whose arithmetic
right rotation differs from standard SM4. Short and long fixture packets are
compared with the official SDK. Unavailable rendering/audio/fonts stay absent,
and the risk-browser indicator remains set. An empty, invalid or expired server
receipt never authorizes a draw. The latest live check returned an empty BSFIT
receipt, including the separately authorized controlled-browser reference; this
must not be presented as complete App attestation or a completed lottery.

`wheel.js` is retained solely as the upstream ABI/reference for offline golden
test generation. It is not embedded or used by the production generator.

`coc3wasm/testdata/device-reference.json` and
`device-confirmed-reference.json` contain only fixture sessions. They compare
the preflight packet and subsequent lottery parameters against the official
JavaScript glue with a fixed clock and entropy in an isolated environment.

`CAIYUN_COC3_WASM_DIR` optionally replaces **only** `wheel_bg.wasm`; a static
header can still be supplied through `CAIYUN_COC3_WASM_PARAMS`.
