import { describe, expect, it } from "vitest";
import { ConnectError, Code } from "@connectrpc/connect";
import { BinaryWriter, WireType } from "@bufbuild/protobuf/wire";
import { requiredSessionLogin } from "./login-required";
function missing(account = "work", key = "creation") {
  const writer = new BinaryWriter()
    .tag(1, WireType.LengthDelimited)
    .string("PROJECT_LOGIN_REQUIRED")
    .tag(2, WireType.LengthDelimited)
    .string("cxz.auth");
  for (const [name, value] of [
    ["account", account],
    ["session_key", key],
  ])
    writer
      .tag(3, WireType.LengthDelimited)
      .bytes(
        new BinaryWriter()
          .tag(1, WireType.LengthDelimited)
          .string(name)
          .tag(2, WireType.LengthDelimited)
          .string(value)
          .finish(),
      );
  const error = new ConnectError("missing profile", Code.FailedPrecondition);
  error.details = [{ type: "google.rpc.ErrorInfo", value: writer.finish() }];
  return error;
}
describe("session login retry", () => {
  it("accepts only a structured login request for this account and creation", () => {
    expect(requiredSessionLogin(missing(), "work", "creation")).toEqual({
      account: "work",
      key: "creation",
    });
    expect(
      requiredSessionLogin(missing("other"), "work", "creation"),
    ).toBeUndefined();
    expect(
      requiredSessionLogin(missing("work", "other"), "work", "creation"),
    ).toBeUndefined();
  });
  it("does not retry a generic auth failure or malformed detail", () => {
    expect(
      requiredSessionLogin(
        new ConnectError("login broken", Code.FailedPrecondition),
        "work",
        "creation",
      ),
    ).toBeUndefined();
    const broken = new ConnectError("login", Code.FailedPrecondition);
    broken.details = [
      { type: "google.rpc.ErrorInfo", value: new Uint8Array([255]) },
    ];
    expect(requiredSessionLogin(broken, "work", "creation")).toBeUndefined();
  });
});
