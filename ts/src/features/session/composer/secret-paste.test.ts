import { expect, it, vi } from "vitest";
import { prepareSecretPaste, type SecretFiles } from "./secret-paste";
import { attachmentsReady, expandPastes } from "./composer-pastes";
import { presentPaste } from "./paste-presentation";
import { readSecretFile } from "./secret-paste";
import { create } from "@bufbuild/protobuf";
import { ProjectDownloadReplySchema } from "#gen/cxz/project_svc_pb";

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

it("reports real progress below 100 until the file is acknowledged and hides internal chip IDs", async () => {
  let finish!: (path: string) => void;
  const { paste, save } = prepareSecretPaste(
    token,
    "GitHub token",
    store({
      put: (_bytes, _signal, progress) => {
        progress?.(50, 100);
        progress?.(100, 100);
        return new Promise((resolve) => {
          finish = resolve;
        });
      },
    }),
  );
  const changed = vi.fn();
  const pending = save(new AbortController().signal, changed);
  expect(paste.secret?.progress).toBe(99);
  expect(presentPaste(paste)).toMatchObject({
    label: "  099%",
    progress: 99,
  });
  expect(changed).toHaveBeenCalledTimes(2);
  finish("/cxz/secrets/fixture/value");
  await pending;
  expect(presentPaste(paste).label).toBe("Secret");
});

it("reads bounded secret bytes via Download without saving a browser file", async () => {
  const chunk = new TextEncoder().encode(" value\n");
  const client = {
    download: vi.fn(async function* () {
      yield create(ProjectDownloadReplySchema, {
        totalSize: BigInt(chunk.length),
      });
      yield create(ProjectDownloadReplySchema, { data: chunk });
    }),
  };
  const bytes = await readSecretFile(
    client,
    "project",
    "/cxz/secrets/fixture/value",
    new AbortController().signal,
  );
  expect(new TextDecoder().decode(bytes)).toBe(" value\n");
  expect(chunk.every((byte) => byte === 0)).toBe(true);
  for (const size of [1n, 65537n]) {
    const bad = {
      download: vi.fn(async function* () {
        yield create(ProjectDownloadReplySchema, { totalSize: size });
      }),
    };
    await expect(
      readSecretFile(bad, "project", "/value", new AbortController().signal),
    ).rejects.toThrow();
  }
});
