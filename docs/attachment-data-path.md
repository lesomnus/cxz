# Attachment data path

The browser and eligible TUI connections upload a raw HTTP request body to the
Manager that owns the attachment store. No protobuf content messages are created
on this path. Session discovery, aliases and every other control operation stay
on the existing RPC API. `SessionService.Upload` remains the compatibility API;
the in-process WASM sandbox continues to use it.

```mermaid
flowchart LR
  Browser -->|raw body, browser cookie| Gateway
  Gateway -->|raw HTTP, private web.sock| Manager
  TUI -->|raw HTTP, local or SSH bridge| Manager
  Manager -->|lifecycle checks, io.Reader| CAS[flob CAS and immutable export]
  Compatibility[Older Manager / TCP / mTLS / WASM] -->|Session.Upload| Manager
```

`HEAD /attachments/` on the private Manager listener advertises
`Cxz-Attachment-Protocol: 1`. `POST /attachments/{runtime-id}` uses `run`, `name`
and `size` query parameters and returns `{"path":"/cxz/assets/..."}`. Metadata
and actual EOF are checked independently. TUI requests use chunked HTTP so an
excess source byte cannot be silently omitted by a fixed Content-Length before
storage decides whether to publish. Browser File bodies retain their native
Content-Length. Limits and filename rules live in `internal/assets/store.go`.

Storage consumes the same reader in both implementations. The lifecycle layer
resolves the session while holding the existing transition lock; the runtime
checks its current run, project and asset mount. The raw route reuses the
version-switch and update-maintenance stream guards for the entire operation.
The private socket keeps its existing owner-only permissions. The web gateway
retains cookie authentication, exact origin checking, body limits and logout
cancellation. No client can supply a project or destination filesystem path.
The resulting hard link is immutable and existing session purge reclaims it.

There are no upload tickets, resumable offsets, sidecar files or extra public
ports. Small legacy paste attachments sent through the separate `Attach` RPC
keep their existing semantics.

## Connection discovery and compatibility

| Connection | Data path |
| --- | --- |
| Browser through updated web gateway | Raw HTTP proxy to Manager's existing private Connect socket |
| Host-local TUI / installed Manager | Private socket, or the existing authenticated Docker exec bridge |
| Explicit conventional Unix daemon socket | Neighboring private Connect socket |
| SSH TUI | Same SSH identity, binary and state, through `_connect-http` |
| TCP or mTLS relay, including enrolled SSH using mTLS | Existing authenticated gRPC Upload |
| Manager without raw endpoint | Existing gRPC Upload |
| In-process WASM sandbox | Existing in-process Upload |

Relays currently forward native gRPC and do not advertise raw HTTP. The client
does not infer another port, obtain a new credential or unexpectedly open SSH
after choosing an enrolled mTLS connection. Adding authenticated raw relay
multiplexing is a separate transport change; preserving the RPC adapter is the
rollout decision here.

Capability discovery happens before consuming upload bytes. A missing endpoint
or absent private socket can select compatibility; authentication failures and
timeouts are surfaced. Once POST begins, failure never retries the reader or
switches to RPC, because publication may have succeeded before the receipt was
lost. Redirects are refused. Closing the owning client cancels active transfers
and idle bridge connections. Blocking producer readers must implement io.Closer
so cancellation can interrupt their Read; successful uploads leave completed
source ownership with the caller.

Updating the UI alone does not update the Manager storage endpoint. Update both
Manager and web gateway to use the raw browser path; newer clients and gateways
retain compatibility with a Manager that still exposes only Upload.

## Measurement

`server/lifecycle/attachments_benchmark_linux_test.go` compares the actual
resource-client gRPC adapter with its raw HTTP adapter, using the same lifecycle,
bounded repeating reader and loopback network. `discard` isolates transport
processing; `storage` adds SHA-256, temporary writes, fsync, CAS deduplication and
immutable publication. Sources are streamed rather than allocated as whole files.
Both connections are warmed before timing, including discovery and TLS handshake.
Storage repetitions use identical content after a warm upload, so they measure
the deduplication/publication case and benefit from the OS page cache.

Measured on Linux/amd64, Go 1.27.1, AMD Ryzen 7 5825U, GOMAXPROCS=2, on
2026-10-12. Each table value is the median of three runs with four operations per
run. The concurrent cases use four workers. These are short local measurements,
not production throughput guarantees. They omit browser, Docker exec and SSH
relay overhead. TLS cases include payload encryption after a trusted test
certificate handshake; production certificate management is not benchmarked.

```sh
TMPDIR=/tmp GOMAXPROCS=2 go test -p 2 ./server/lifecycle -run '^$' \
  -bench BenchmarkAttachmentDataPath -benchtime=4x -count=3 -benchmem
```

Representative storage results (throughput uses decimal MB/s):

| File | Workers | TLS | gRPC MB/s | Raw MB/s | Improvement |
| --- | ---: | --- | ---: | ---: | ---: |
| 32 KiB | 1 | No | 27.05 | 35.63 | 1.32× |
| 8 MiB | 1 | No | 363.78 | 552.03 | 1.52× |
| 64 MiB | 1 | No | 528.68 | 849.46 | 1.61× |
| 64 MiB | 4 | No | 574.97 | 1213.34 | 2.11× |
| 32 KiB | 1 | Yes | 28.62 | 34.14 | 1.19× |
| 8 MiB | 1 | Yes | 349.71 | 514.81 | 1.47× |
| 64 MiB | 1 | Yes | 454.56 | 721.25 | 1.59× |
| 64 MiB | 4 | Yes | 479.21 | 928.52 | 1.94× |

The main result is reduced byte processing and allocation, rather than wire
framing savings. For the unencrypted single-worker 64 MiB case:

| Metric | gRPC | Raw HTTP |
| --- | ---: | ---: |
| Process CPU per upload | 231.16 ms | 100.06 ms |
| Allocated bytes per upload | 131.72 MiB | 0.13 MiB |
| Allocation count per upload | 8715 | 3374 |
| GC cycles per upload | 11 | 0 |
| Sampled heap growth across run | 18.64 MiB | 0.51 MiB |

For four workers with TLS, process CPU per upload was 278.88 ms versus 145.97 ms,
allocation was 135.05 MiB versus 0.36 MiB, and sampled heap growth was 44.58 MiB
versus 1.42 MiB. CPU combines user and system time from process rusage across all
workers. Heap growth samples HeapAlloc relative to the post-GC warm baseline;
it excludes baseline memory, stacks, kernel buffers and OS page cache, and may
miss peaks between samples. It is not peak RSS or a memory limit. Raw HTTP still
copies buffers, hashes data and encrypts TLS records; it is not zero-copy.

These results support the raw path for eligible uploads while retaining RPC for
transport compatibility. More extensive production/relay and cold-storage
measurements should precede any decision to remove Upload.

## Validation

- Raw HTTP storage tests cover binary/Unicode names, empty files, short and
  excessive streams, stale runs, cancellation, immutable publication and purge.
- Lifecycle/resource-client tests cover runtime IDs, aliases, unknown sessions,
  transaction restrictions, missing endpoint fallback and no replay after a
  dropped POST receipt.
- The private-socket transport test verifies local installation and conventional
  Unix selection, and refuses invented TCP/mTLS endpoints.
- Web tests cover cookie/origin authentication, large raw bodies without invoking
  Upload RPC, legacy RPC compatibility and logout during a stalled upload.
- The browser file-drop test uses the real private HTTP proxy and storage reader;
  it checks returned paths and the attachment reference sent to the conversation.
