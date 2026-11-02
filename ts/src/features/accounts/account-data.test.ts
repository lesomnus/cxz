import { create } from "@bufbuild/protobuf";
import { expect, it, vi } from "vitest";
import { AccountSchema } from "#gen/cxz/account_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import {
  editAccount,
  sessionsForAccount,
  validateAccountField,
} from "./account-data";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

it("edits only the selected account field with a stable id and checked version", async () => {
  const account = create(AccountSchema, {
    id: new Uint8Array(16).fill(1),
    alias: "work",
    name: "Work",
    dateUpdated: timestampFromDate(new Date("2026-10-20T00:00:00Z")),
  });
  const client = {
    get: vi.fn().mockResolvedValue(account),
    patch: vi.fn().mockResolvedValue(account),
  };
  await editAccount(client, account, "alias", "renamed");
  expect(client.patch).toHaveBeenCalledWith(
    {
      ref: { key: { case: "id", value: account.id } },
      alias: "renamed",
      dateUpdated: account.dateUpdated,
    },
    expect.anything(),
  );
  expect(client.get).not.toHaveBeenCalled();
  account.dateUpdated = undefined;
  await expect(
    editAccount(client, account, "name", "New name"),
  ).rejects.toThrow("version is unavailable");
  expect(client.patch).toHaveBeenCalledTimes(1);
});

it("validates UTF-8 names and account aliases without sending invalid edits", async () => {
  expect(validateAccountField("name", "가".repeat(66))).toBeUndefined();
  expect(validateAccountField("name", "가".repeat(67))).toBeTruthy();
  expect(validateAccountField("name", "x\ny")).toBeTruthy();
  expect(validateAccountField("alias", "a")).toBeUndefined();
  expect(validateAccountField("alias", "a".repeat(64))).toBeTruthy();
  const client = { get: vi.fn(), patch: vi.fn() };
  await expect(
    editAccount(client, create(AccountSchema), "alias", "Bad_Alias"),
  ).rejects.toThrow("lowercase");
  expect(client.get).not.toHaveBeenCalled();
  expect(client.patch).not.toHaveBeenCalled();
});

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
