import { Code, ConnectError } from "@connectrpc/connect";
import type { Connection } from "#src/shared/api/connection.ts";
import type { Account } from "#gen/cxz/account_pb";
import type { Session } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";

export type AccountField = "name" | "alias";
export function validateAccountField(field: AccountField, value: string) {
  if (field === "name") {
    if (
      !value.trim() ||
      new TextEncoder().encode(value).length > 200 ||
      /[\r\n\t\0]/.test(value)
    )
      return t(
        "Display name must be nonempty, at most 200 bytes and contain no control whitespace.",
      );
  } else if (!/^[a-z][a-z0-9-]{0,62}$/.test(value)) {
    return "Account alias must be 1–63 lowercase letters, digits or hyphens, starting with a letter.";
  }
}

export async function editAccount(
  client: Pick<Connection["accounts"], "get" | "patch">,
  account: Account,
  field: AccountField,
  value: string,
) {
  const invalid = validateAccountField(field, value);
  if (invalid) throw new Error(invalid);
  const ref = account.id.length
    ? { key: { case: "id" as const, value: account.id } }
    : accountRef(account.alias);
  const options = { signal: AbortSignal.timeout(20_000) };
  const current = account.dateUpdated
    ? account
    : await client.get({ ref, select: { all: true } }, options);
  if (!current.dateUpdated)
    throw new Error("Account version is unavailable. Refresh and try again.");
  return client.patch(
    { ref, dateUpdated: current.dateUpdated, [field]: value },
    options,
  );
}

export function sessionsForAccount(
  account: Pick<Account, "id" | "alias">,
  sessions: Session[],
) {
  return sessions.filter((session) => {
    const linked = session.account;
    if (!linked) return false;
    if (account.id.length && linked.id.length)
      return (
        account.id.length === linked.id.length &&
        account.id.every((value, index) => value === linked.id[index])
      );
    return !!linked.alias && linked.alias === account.alias;
  });
}
export const accountRef = (alias: string) => ({
  key: { case: "alias" as const, value: alias },
});
export async function listAccounts(
  c: Pick<Connection, "accounts">,
  signal?: AbortSignal,
) {
  const items = [];
  let after = "";
  do {
    const page = await c.accounts.list(
      { size: 200, after },
      { signal, timeoutMs: 10000 },
    );
    items.push(...page.items);
    if (page.next && page.next === after)
      throw new Error("Account cursor did not advance.");
    after = page.next;
  } while (after);
  return items;
}
export async function accountStatus(
  c: Connection,
  alias: string,
  signal: AbortSignal,
) {
  try {
    return await c.accounts.status(accountRef(alias), {
      signal,
      timeoutMs: 10000,
    });
  } catch (error) {
    if (ConnectError.from(error).code === Code.Unimplemented)
      throw new ConnectError(
        "Login status unavailable. Update the Manager.",
        Code.Unimplemented,
        undefined,
        undefined,
        error,
      );
    throw error;
  }
}
