import { describe, expect, it } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import { trustRequired } from "./config-trust";

// What internal/configtrust sends: the findings between a fixed first and
// last line, and an ErrorInfo that says which question this is.
const message = [
  "devcontainer configuration requires explicit trust:",
  'configuration "/w/.devcontainer/docker-compose.yaml":  - "/services/dev/privileged": privileged container requested',
  'configuration merged devcontainer.compose:  - "/services/dev/volumes/5/source": Docker socket reference detected',
  "inspect these settings and explicitly pass --trust-config if trusted (configuration values omitted)",
].join("\n");

function refused(reason = "TRUST_REQUIRED", domain = "cxz.config") {
  const error = new ConnectError(message, Code.FailedPrecondition);
  error.details = [
    {
      type: "google.rpc.ErrorInfo",
      value: new BinaryWriter()
        .tag(1, WireType.LengthDelimited)
        .string(reason)
        .tag(2, WireType.LengthDelimited)
        .string(domain)
        .finish(),
    },
  ];
  return error;
}

describe("configuration trust", () => {
  it("lists the findings a person is asked to accept", () => {
    expect(trustRequired(refused())).toEqual([
      'configuration "/w/.devcontainer/docker-compose.yaml":  - "/services/dev/privileged": privileged container requested',
      'configuration merged devcontainer.compose:  - "/services/dev/volumes/5/source": Docker socket reference detected',
    ]);
  });
  it("asks only on the structured signal", () => {
    expect(
      trustRequired(new ConnectError(message, Code.FailedPrecondition)),
    ).toBeUndefined();
    expect(trustRequired(refused("PROJECT_LOGIN_REQUIRED"))).toBeUndefined();
    expect(
      trustRequired(refused("TRUST_REQUIRED", "cxz.auth")),
    ).toBeUndefined();
    expect(trustRequired(new Error(message))).toBeUndefined();
  });
});
