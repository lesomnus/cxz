import type { SessionEvent } from "#gen/cxz/session_pb";
import type { Question } from "./journal";
import type { SessionDrafts } from "./session-drafts";

export type QuestionDraft = {
  selected: Record<string, string[]>;
  other: Record<string, string>;
  step: number;
};
export const emptyQuestionDraft = (): QuestionDraft => ({
  selected: {},
  other: {},
  step: 0,
});

function requestKey(session: string, event: SessionEvent) {
  return JSON.stringify(["question", session, event.runId, event.requestId]);
}
function answerKey(request: string, question: string) {
  return JSON.stringify([request, question]);
}
function schema(questions: readonly Question[]) {
  return JSON.stringify(
    questions.map((q) => [
      q.key,
      q.text,
      q.multi,
      q.other,
      q.secret,
      q.options.map((o) => o.label),
    ]),
  );
}
type Metadata = {
  schema: string;
  selected: Record<string, string[]>;
  step: number;
  keys: string[];
};
function metadata(text?: string): Metadata | undefined {
  try {
    const value: unknown = text ? JSON.parse(text) : undefined;
    if (!value || typeof value !== "object") return;
    const v = value as Partial<Metadata>;
    if (
      typeof v.schema !== "string" ||
      !v.selected ||
      typeof v.selected !== "object" ||
      Array.isArray(v.selected) ||
      !Number.isSafeInteger(v.step) ||
      !Array.isArray(v.keys) ||
      !v.keys.every((key) => typeof key === "string")
    )
      return;
    return v as Metadata;
  } catch {
    return;
  }
}

/** Shares the composer's tab-local storage and chip serialization, scoped per request. */
export class QuestionDrafts {
  constructor(private drafts: SessionDrafts) {}

  get(
    session: string,
    event: SessionEvent,
    questions: readonly Question[],
  ): QuestionDraft {
    const request = requestKey(session, event);
    const saved = metadata(this.drafts.get(request));
    if (!saved) return emptyQuestionDraft();
    if (saved.schema !== schema(questions)) {
      this.delete(session, event);
      return emptyQuestionDraft();
    }
    const selected = Object.fromEntries(
      questions
        .filter((q) => !q.secret)
        .map((q) => {
          const choices = saved.selected[q.key];
          const values = Array.isArray(choices)
            ? [
                ...new Set(
                  choices.filter((value) =>
                    q.options.some((o) => o.label === value),
                  ),
                ),
              ]
            : [];
          return [q.key, q.multi ? values : values.slice(0, 1)];
        }),
    );
    const other = Object.fromEntries(
      questions
        .filter((q) => q.other && !q.secret)
        .map((q) => [q.key, this.drafts.get(answerKey(request, q.key)) ?? ""]),
    );
    return {
      selected,
      other,
      step: Math.max(0, Math.min(saved.step, questions.length - 1)),
    };
  }

  set(
    session: string,
    event: SessionEvent,
    questions: readonly Question[],
    value: QuestionDraft,
  ) {
    const request = requestKey(session, event);
    const keys = questions
      .filter((q) => q.other && !q.secret)
      .map((q) => q.key);
    const selected = Object.fromEntries(
      questions
        .filter((q) => !q.secret)
        .map((q) => [q.key, value.selected[q.key] ?? []]),
    );
    const text = JSON.stringify({
      schema: schema(questions),
      selected,
      step: value.step,
      keys,
    });
    if (this.drafts.get(request) !== text) this.drafts.set(request, text);
    for (const key of keys) {
      const id = answerKey(request, key),
        answer = value.other[key] ?? "";
      if (this.drafts.get(id) !== answer) this.drafts.set(id, answer);
    }
  }

  persistAttachments(session: string, event: SessionEvent, question: string) {
    this.drafts.persist(answerKey(requestKey(session, event), question));
  }

  delete(session: string, event: SessionEvent) {
    const request = requestKey(session, event);
    for (const key of metadata(this.drafts.get(request))?.keys ?? [])
      this.drafts.delete(answerKey(request, key));
    this.drafts.delete(request);
  }
}
