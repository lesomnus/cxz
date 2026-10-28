import { expect, it, vi } from "vitest";
import { prepareSecretPaste, type SecretFiles } from "./secret-paste";
import { attachmentsReady, expandPastes } from "./composer-pastes";

const token = `ghp_${"aB12".repeat(9)}`;
function store(overrides: Partial<SecretFiles> = {}): SecretFiles {
  return {
    session: "session",
    run: "run",
    put: async () => "/cxz/secrets/fixture/value",
    remove: async () => {},
    ...overrides,
  };
}

it("keeps raw bytes out of the chip and prompt while pending, then clears the byte buffer", async () => {
  let finish!: (path: string) => void;
  let bytes!: Uint8Array;
  const { paste, save } = prepareSecretPaste(
    ` ${token}\n`,
    "GitHub token",
    store({
      put: (body) => {
        bytes = body;
        return new Promise((resolve) => {
          finish = resolve;
        });
      },
    }),
  );
  const pastes = new Map([[paste.token, paste]]);
  const saving = save(new AbortController().signal);
  expect(new TextDecoder().decode(bytes)).toBe(` ${token}\n`);
  expect(JSON.stringify(paste)).not.toContain(token);
  expect(expandPastes(paste.token, pastes)).toBe(paste.token);
  expect(attachmentsReady(paste.token, pastes)).toBe(false);
  finish("/cxz/secrets/fixture/value");
  await saving;
  expect(bytes.every((byte) => byte === 0)).toBe(true);
  expect(paste.secret).toMatchObject({
    state: "ready",
    session: "session",
    run: "run",
  });
  expect(expandPastes(paste.token, pastes)).toBe(
    "(secret placed at /cxz/secrets/fixture/value)",
  );
  expect(attachmentsReady(paste.token, pastes)).toBe(true);
});

it("sanitizes failures and never makes the failed chip sendable", async () => {
  let bytes!: Uint8Array;
  const { paste, save } = prepareSecretPaste(
    token,
    "GitHub token",
    store({
      put: async (body) => {
        bytes = body;
        throw new Error(`server echoed ${token}`);
      },
    }),
  );
  await save(new AbortController().signal);
  expect(JSON.stringify(paste)).not.toContain(token);
  expect(paste.secret?.state).toBe("error");
  expect(attachmentsReady(paste.token, new Map([[paste.token, paste]]))).toBe(
    false,
  );
  expect(bytes.every((byte) => byte === 0)).toBe(true);
});

it("cleans up a file acknowledged after cancellation", async () => {
  const controller = new AbortController();
  const remove = vi.fn(async () => {});
  const { paste, save } = prepareSecretPaste(
    token,
    "GitHub token",
    store({
      put: async () => {
        controller.abort();
        return "/cxz/secrets/fixture/value";
      },
      remove,
    }),
  );
  await save(controller.signal);
  expect(remove).toHaveBeenCalledWith("/cxz/secrets/fixture/value");
  expect(paste.secret).toMatchObject({ state: "error", path: "" });
});

it("refuses missing sessions and oversized secrets without invoking ordinary storage", async () => {
  const missing = prepareSecretPaste(token, "GitHub token");
  await missing.save(new AbortController().signal);
  expect(missing.paste.secret?.error).toBe(
    "Secret paste requires an active session.",
  );
  const put = vi.fn();
  const large = prepareSecretPaste("x".repeat(65537), "JWT", store({ put }));
  await large.save(new AbortController().signal);
  expect(large.paste.secret?.error).toBe("Secret limit: 64 KiB");
  expect(put).not.toHaveBeenCalled();
});
