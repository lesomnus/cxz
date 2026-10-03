# Mobile web implementation

This implementation follows the feature specification and mobile plan in PR #78.
It is developed in `feat/mobile-web-client` and published incrementally as a
Draft PR; the checklist records implemented work, not promised support.

## Architecture

- Reuse cxz's payday ProjectService and SessionService protobuf contracts.
- Use `github.com/lesomnus/payday/web` to expose Connect / gRPC-Web over HTTPS.
  Browsers use generated TypeScript descriptors and Connect transport, not
  native HTTP/2 gRPC or a separate REST conversation API.
- Serve the UI from the same HTTPS origin. Keep the client transport behind a
  Connection boundary so later deployments can choose another HTTPS gateway.
- Initially run an explicit gateway against the installed Manager. Gateway
  shutdown or browser suspension must not terminate agents or projects.
- Authenticate browser access separately from model-provider accounts; require
  HTTPS and an explicit allowed origin. Never expose an unauthenticated port.
- Reuse journal History/Events cursors, run IDs and mutation client IDs; never
  silently replay a send after an ambiguous disconnect.

## Incremental commits

- [x] Confirm payday transport support and record implementation boundaries.
- [x] HTTPS gateway, browser authentication and Connect integration tests.
- [ ] Generated TypeScript client and mobile project/session navigation.
- [ ] Conversation history/streaming, sending, approvals and questions.
- [ ] Reconnection, mobile checks, build/CI and usage documentation.

The first release targets private VPN HTTPS and one same-origin connection.
Native apps, external hosting/multiple connections, uploads, downloads and
full Settings parity remain follow-up work. Support documentation must label
these gaps explicitly.
