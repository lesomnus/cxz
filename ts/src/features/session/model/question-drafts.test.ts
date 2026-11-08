import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import { SessionEventSchema } from "#gen/cxz/session_pb";
import type { Question } from "./journal";
import { QuestionDrafts } from "./question-drafts";
import { SessionDrafts } from "./session-drafts";
import {
  attachmentsReady,
  createPaste,
  expandPastes,
  type ComposerPaste,
} from "../composer/composer-pastes";

function storage() {
  const rows = new Map<string, string>();
  return {
    getItem: (key: string) => rows.get(key) ?? null,
    setItem: (key: string, value: string) => {
      rows.set(key, value);
    },
    removeItem: (key: string) => {
      rows.delete(key);
    },
    key: (index: number) => [...rows.keys()][index] ?? null,
    get length() {
      return rows.size;
    },
    clear: () => rows.clear(),
  } satisfies Storage;
}
const event = (runId = "run", requestId = "request") =>
  create(SessionEventSchema, { runId, requestId });
const questions: Question[] = [
  {
    key: "choice",
    text: "Choose",
    options: [{ label: "One" }, { label: "Two" }],
    multi: true,
    other: false,
    secret: false,
  },
  {
    key: "notes",
    text: "Notes",
    options: [],
    multi: false,
    other: true,
    secret: false,
  },
];
const answer = {
  selected: { choice: ["One", "Two"], notes: [] },
  other: { notes: "한글\n```ts\nconst tab = 4;\n```" },
  step: 1,
};

it("restores choices, multiline text and the selected question after navigation and reload", () => {
  const file = storage();
  const drafts = new SessionDrafts("host", new Map(), () => file);
  new QuestionDrafts(drafts).set("oak", event(), questions, answer);
  expect(new QuestionDrafts(drafts).get("oak", event(), questions)).toEqual(
    answer,
  );
  expect(
    new QuestionDrafts(new SessionDrafts("host", new Map(), () => file)).get(
      "oak",
      event(),
      questions,
    ),
  ).toEqual(answer);
});

it("isolates connections, sessions, runs and request IDs from each other and the composer", () => {
  const file = storage();
  const drafts = new SessionDrafts("host", new Map(), () => file),
    saved = new QuestionDrafts(drafts);
  drafts.set("oak", "Composer draft");
  saved.set("oak", event(), questions, answer);
  for (const [session, request] of [
    ["elm", event()],
    ["oak", event("other")],
    ["oak", event("run", "other")],
  ] as const)
    expect(saved.get(session, request, questions).step).toBe(0);
  expect(
    new QuestionDrafts(
      new SessionDrafts("other-host", new Map(), () => file),
    ).get("oak", event(), questions).other,
  ).toEqual({});
  expect(drafts.get("oak")).toBe("Composer draft");
});

it("restores paste chips and completed attachment metadata using the existing draft serializer", () => {
  const file = storage(),
    pastes = new Map<string, ComposerPaste>();
  const paste = createPaste("Full attached answer\nLine two", "12345678");
  paste.attachment = {
    name: 'notes "final".txt',
    state: "uploading",
    file: new File([paste.body], 'notes "final".txt'),
  };
  paste.sendAs = "file";
  pastes.set(paste.token, paste);
  const saved = new QuestionDrafts(
    new SessionDrafts("host", pastes, () => file),
  );
  saved.set("oak", event(), questions, {
    ...answer,
    other: { notes: `Read ${paste.token}` },
  });
  const interrupted = new Map<string, ComposerPaste>();
  const interruptedDraft = new QuestionDrafts(
    new SessionDrafts("host", interrupted, () => file),
  ).get("oak", event(), questions);
  expect(attachmentsReady(interruptedDraft.other.notes, interrupted)).toBe(
    false,
  );
  paste.attachment.state = "ready";
  paste.attachment.path = "/cxz/assets/notes.txt";
  saved.persistAttachments("oak", event(), "notes");
  const restored = new Map<string, ComposerPaste>();
  const reloaded = new QuestionDrafts(
    new SessionDrafts("host", restored, () => file),
  ).get("oak", event(), questions);
  expect(attachmentsReady(reloaded.other.notes, restored)).toBe(true);
  expect(expandPastes(reloaded.other.notes, restored)).toContain(
    "/cxz/assets/notes.txt",
  );
  expect(restored.get(paste.token)?.body).toBe(paste.body);
});

it("removes only the confirmed request and clears question data with normal sign-out cleanup", () => {
  const file = storage(),
    drafts = new SessionDrafts("host", new Map(), () => file),
    saved = new QuestionDrafts(drafts);
  drafts.set("oak", "Composer draft");
  saved.set("oak", event(), questions, answer);
  saved.set("oak", event("run", "second"), questions, answer);
  saved.delete("oak", event());
  const reload = new QuestionDrafts(
    new SessionDrafts("host", new Map(), () => file),
  );
  expect(reload.get("oak", event(), questions).other).toEqual({});
  expect(reload.get("oak", event("run", "second"), questions)).toEqual(answer);
  expect(drafts.get("oak")).toBe("Composer draft");
  drafts.clear();
  expect(file.length).toBe(0);
});

it("does not persist password answers and rejects stale question definitions", () => {
  const file = storage(),
    saved = new QuestionDrafts(
      new SessionDrafts("host", new Map(), () => file),
    );
  const secret = { ...questions[1], key: "password", secret: true };
  saved.set("oak", event(), [...questions, secret], {
    ...answer,
    other: { ...answer.other, password: "private-answer" },
  });
  const contents = Array.from({ length: file.length }, (_, i) =>
    file.getItem(file.key(i)!),
  ).join("");
  expect(contents).not.toContain("private-answer");
  expect(
    saved.get("oak", event(), [...questions, secret]).other.password,
  ).toBeUndefined();
  expect(
    saved.get("oak", event(), [{ ...questions[0], text: "A new question" }])
      .selected,
  ).toEqual({});
  expect(file.length).toBe(0);
});

it("keeps in-memory navigation drafts usable when browser storage is denied", () => {
  const drafts = new SessionDrafts("host", new Map(), () => {
    throw Error("blocked");
  });
  new QuestionDrafts(drafts).set("oak", event(), questions, answer);
  expect(new QuestionDrafts(drafts).get("oak", event(), questions)).toEqual(
    answer,
  );
  expect(() => new QuestionDrafts(drafts).delete("oak", event())).not.toThrow();
});
