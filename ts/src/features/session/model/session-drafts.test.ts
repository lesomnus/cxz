import { expect, it } from "vitest";
import { SessionDrafts } from "./session-drafts";
import { restoreEditorSelection } from "#src/features/session/composer/editor-selection.ts";
import {
  createPaste,
  createFileChip,
  isImagePaste,
  expandPastes,
  attachmentsReady,
  type ComposerPaste,
} from "../composer/composer-pastes";
import { prepareSecretPaste } from "../composer/secret-paste";

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

it("persists only a secret placeholder and restores interrupted preparation as unsendable", () => {
  const file = storage();
  const raw = `ghp_${"aB12".repeat(9)}`;
  const { paste } = prepareSecretPaste(raw, "GitHub token");
  paste.secret!.listeners = new Set([() => {}]);
  const drafts = new SessionDrafts(
    "host",
    new Map([[paste.token, paste]]),
    () => file,
  );
  drafts.set("one", paste.token);
  const saved = file.getItem("cxz.draft.v1:host:one")!;
  expect(saved).not.toContain(raw);
  expect(saved).not.toContain("listeners");
  const restored = new Map<string, ComposerPaste>();
  const text = new SessionDrafts("host", restored, () => file).get("one")!;
  expect(text).toBe(paste.token);
  expect(restored.get(paste.token)?.secret?.state).toBe("error");
  expect(attachmentsReady(text, restored)).toBe(false);
});
it("restores tab-local cursor ranges per host/session without rewriting draft contents", () => {
  const file = storage();
  const drafts = new SessionDrafts("cursor-host", new Map(), () => file);
  drafts.set("one", "A long draft");
  drafts.set("two", "Other text");
  const before = file.getItem("cxz.draft.v1:cursor-host:one");
  drafts.setSelection("one", {
    start: 2,
    end: 7,
    direction: "backward",
    scrollTop: 24,
    scrollLeft: 0,
  });
  drafts.setSelection("two", {
    start: 3,
    end: 3,
    direction: "none",
    scrollTop: 0,
    scrollLeft: 0,
  });
  expect(file.getItem("cxz.draft.v1:cursor-host:one")).toBe(before);
  const refreshed = new SessionDrafts("cursor-host", new Map(), () => file);
  expect(refreshed.selection("one")).toEqual({
    start: 2,
    end: 7,
    direction: "backward",
    scrollTop: 24,
    scrollLeft: 0,
  });
  expect(refreshed.selection("two")?.start).toBe(3);
  expect(
    new SessionDrafts("another-host", new Map(), () => file).selection("one"),
  ).toBeUndefined();
  expect(restoreEditorSelection(refreshed.selection("one"), 4)).toMatchObject({
    start: 2,
    end: 4,
  });
  refreshed.set("one", "");
  expect(refreshed.selection("one")).toBeUndefined();
  refreshed.clear();
  expect(file.length).toBe(0);
});
it("rejects corrupt cursor records and tolerates blocked cursor storage", () => {
  const file = storage();
  file.setItem(
    "cxz.draft.v1:host:selection:one",
    JSON.stringify({
      start: -1,
      end: 3,
      direction: "none",
      scrollTop: 0,
      scrollLeft: 0,
    }),
  );
  expect(
    new SessionDrafts("host", new Map(), () => file).selection("one"),
  ).toBeUndefined();
  expect(restoreEditorSelection(undefined, 5)).toMatchObject({
    start: 5,
    end: 5,
  });
  const drafts = new SessionDrafts("blocked", new Map(), () => {
    throw new Error("blocked");
  });
  drafts.set("one", "text");
  drafts.setSelection("one", {
    start: 1,
    end: 1,
    direction: "none",
    scrollTop: 0,
    scrollLeft: 0,
  });
  expect(drafts.selection("one")?.start).toBe(1);
  expect(() => drafts.clear()).not.toThrow();
});
it("restores secret chips with only a file reference and run binding", () => {
  const file = storage();
  const pastes = new Map<string, ComposerPaste>();
  const chip: ComposerPaste = {
    token: "[Secret 12345678]",
    body: "(secret placed at /run/cxz-secrets/file)",
    lines: 0,
    bytes: 0,
    secret: { session: "one", run: "run", path: "/run/cxz-secrets/file" },
  };
  pastes.set(chip.token, chip);
  new SessionDrafts("host", pastes, () => file).set(
    "one",
    `Read ${chip.token}`,
  );
  const restored = new Map<string, ComposerPaste>();
  const drafts = new SessionDrafts("host", restored, () => file);
  expect(expandPastes(drafts.get("one")!, restored)).toBe(
    "Read (secret placed at /run/cxz-secrets/file)",
  );
  expect(restored.get(chip.token)?.secret).toEqual(chip.secret);
});
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
  paste.sendAs = "text";
  pastes.set(paste.token, paste);
  const drafts = new SessionDrafts("host", pastes, () => file);
  const text = `Before ${paste.token} after`;
  drafts.set("one", text);
  const restored = new Map<string, ComposerPaste>();
  const reload = new SessionDrafts("host", restored, () => file);
  expect(reload.get("one")).toBe(text);
  expect(restored.get(paste.token)?.body).toBe(paste.body);
  expect(attachmentsReady(text, restored)).toBe(true);
  expect(restored.get(paste.token)?.sendAs).toBe("text");
  expect(expandPastes(text, restored)).toBe("Before " + paste.body + " after");
  restored.get(paste.token)!.sendAs = "file";
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

it("preserves image type after reload without persisting the file blob or requiring an extension", () => {
  const file = storage();
  const image = createFileChip(
    new File(["image bytes"], "clipboard", { type: "image/png" }),
    "clipboard",
  );
  image.attachment!.state = "ready";
  image.attachment!.path = "/cxz/assets/clipboard";
  const drafts = new SessionDrafts(
    "host",
    new Map([[image.token, image]]),
    () => file,
  );
  drafts.set("one", image.token);
  const restored = new Map<string, ComposerPaste>();
  const reload = new SessionDrafts("host", restored, () => file);
  expect(reload.get("one")).toBe(image.token);
  const paste = restored.get(image.token)!;
  expect(paste.attachment?.file).toBeUndefined();
  expect(paste.attachment?.mediaType).toBe("image/png");
  expect(isImagePaste(paste)).toBe(true);
  expect(expandPastes(image.token, restored)).toContain(
    "/cxz/assets/clipboard",
  );
  expect(
    isImagePaste({
      ...paste,
      attachment: {
        ...paste.attachment!,
        mediaType: undefined,
        name: "IMAGE.PNG",
      },
    }),
  ).toBe(true);
  expect(
    isImagePaste({
      ...paste,
      attachment: { ...paste.attachment!, directory: true },
    }),
  ).toBe(false);
});
it("storage denial does not break in-memory editing", () => {
  const drafts = new SessionDrafts("host", new Map(), () => {
    throw Error("denied");
  });
  drafts.set("one", "Keep typing");
  expect(drafts.get("one")).toBe("Keep typing");
  expect(() => drafts.clear()).not.toThrow();
});
