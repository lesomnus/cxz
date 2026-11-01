import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { AccountSchema } from "#gen/cxz/account_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { sessionsForAccount } from "./account-data";

it("matches account ids from the unexpanded session inventory", () => {
  const account = create(AccountSchema, {
    id: new Uint8Array([1, 2]),
    alias: "work",
  });
  const match = create(SessionSchema, {
    runtimeId: "match",
    account: { id: new Uint8Array([1, 2]) },
  });
  const other = create(SessionSchema, {
    runtimeId: "other",
    account: { id: new Uint8Array([1, 3]), alias: "work" },
  });
  expect(
    sessionsForAccount(account, [match, other, create(SessionSchema)]),
  ).toEqual([match]);
});

it("supports alias-only legacy references without matching empty ids or aliases", () => {
  const account = create(AccountSchema, {
    id: new Uint8Array([1]),
    alias: "work",
  });
  const match = create(SessionSchema, {
    runtimeId: "match",
    account: { alias: "work" },
  });
  expect(
    sessionsForAccount(account, [
      match,
      create(SessionSchema, { account: {} }),
    ]),
  ).toEqual([match]);
  expect(
    sessionsForAccount(create(AccountSchema), [
      create(SessionSchema, { account: {} }),
    ]),
  ).toEqual([]);
});
