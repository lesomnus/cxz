import { expect, it } from "vitest";
import { SessionDrafts } from "./session-drafts";
import {
  createPaste,
  expandPastes,
  attachmentsReady,
  type ComposerPaste,
} from "../composer/composer-pastes";

function storage() {
  const data = new Map<string, string>();
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => {
      data.set(k, v);
    },
    removeItem: (k: string) => {
      data.delete(k);
    },
    key: (i: number) => [...data.keys()][i] ?? null,
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
  } satisfies Storage;
}
it("restores session-scoped Markdown after refresh and clears accepted drafts and sign-out data", () => {
  const file = storage(),
    pastes = new Map();
  const first = new SessionDrafts("host-a", pastes, () => file);
  first.set("one", "한글\n```js\nconst a = 1;\n```");
  first.set("two", "Other session");
  const reloaded = new SessionDrafts("host-a", new Map(), () => file);
  expect(reloaded.get("one")).toBe(first.get("one"));
  expect(reloaded.get("two")).toBe("Other session");
  expect(
    new SessionDrafts("host-b", pastes, () => file).get("one"),
  ).toBeUndefined();
  reloaded.set("one", "");
  expect(
    new SessionDrafts("host-a", pastes, () => file).get("one"),
  ).toBeUndefined();
  reloaded.clear();
  expect(file.length).toBe(0);
});
it("restores paste bodies and completed attachment paths without serializing file blobs", () => {
  const file = storage(),
    pastes = new Map<string, ComposerPaste>();
  const paste = createPaste(
    "Long original pasted source\nLine two",
    "12345678",
  );
  paste.attachment = {
    file: new File([paste.body], "paste.txt"),
    name: "paste.txt",
    state: "ready",
    path: "/cxz/assets/paste.txt",
  };
  pastes.set(paste.token, paste);
  const drafts = new SessionDrafts("host", pastes, () => file);
  const text = `Before ${paste.token} after`;
  drafts.set("one", text);
  const restored = new Map<string, ComposerPaste>();
  const reload = new SessionDrafts("host", restored, () => file);
  expect(reload.get("one")).toBe(text);
  expect(restored.get(paste.token)?.body).toBe(paste.body);
  expect(attachmentsReady(text, restored)).toBe(true);
  expect(expandPastes(text, restored)).toContain("/cxz/assets/paste.txt");
});
it("keeps interrupted file chips blocked instead of uploading an empty restored file", () => {
  const file = storage(),
    pastes = new Map<string, ComposerPaste>();
  pastes.set("[File 12345678 · report.txt]", {
    token: "[File 12345678 · report.txt]",
    body: "",
    bytes: 999,
    lines: 0,
    attachment: {
      name: "report.txt",
      file: new File(["data"], "report.txt"),
      state: "uploading",
    },
  });
  new SessionDrafts("host", pastes, () => file).set(
    "one",
    "[File 12345678 · report.txt]",
  );
  const restored = new Map<string, ComposerPaste>();
  new SessionDrafts("host", restored, () => file).get("one");
  const paste = restored.get("[File 12345678 · report.txt]")!;
  expect(paste.attachment?.state).toBe("error");
  expect(paste.attachment?.file).toBeUndefined();
  expect(attachmentsReady(paste.token, restored)).toBe(false);
});
it("storage denial does not break in-memory editing", () => {
  const drafts = new SessionDrafts("host", new Map(), () => {
    throw Error("denied");
  });
  drafts.set("one", "Keep typing");
  expect(drafts.get("one")).toBe("Keep typing");
  expect(() => drafts.clear()).not.toThrow();
});
