import { Code, ConnectError } from "@connectrpc/connect";
import type { Connection } from "#src/shared/api/connection.ts";
export const accountRef = (alias: string) => ({
  key: { case: "alias" as const, value: alias },
});
export async function listAccounts(c: Connection, signal?: AbortSignal) {
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
      throw new Error("Login status unavailable. Update the Manager.");
    throw error;
  }
}
