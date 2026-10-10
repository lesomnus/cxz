import { useId, useState } from "react";
import type { SessionEvent } from "../gen/cxz/session_pb";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import { Button } from "./button";
import { FloatingCard, useFloatingCard } from "./floating-card";
import { approvalTitle, questions, payload, detail } from "./journal";
import { ComposerEditor } from "./composer-editor";
import type { ComposerPaste } from "./composer-pastes";
import { composerPrompt } from "./composer-code";
import { TabList } from "./tab-list";
import { Markdown } from "./markdown";

export function QuestionCard({
  e,
  agent,
  pastes,
  busy,
  reply,
}: {
  e: SessionEvent;
  agent: string;
  pastes: Map<string, ComposerPaste>;
  busy: boolean;
  reply: (e: SessionEvent, allow: boolean, answers?: string) => Promise<void>;
}) {
  useLocale();
  const openCard = useFloatingCard();
  const qs = questions(agent, e);
  const [selected, setSelected] = useState<Record<string, string[]>>({});
  const [other, setOther] = useState<Record<string, string>>({});
  const [step, setStep] = useState(0);
  const prefix = useId();
  const activeStep = Math.min(step, qs.length - 1);
  const panel = `${prefix}-panel`;
  const otherAnswer = (key: string) => composerPrompt(other[key] ?? "", pastes);
  const answered = (key: string) =>
    !!(selected[key]?.length || otherAnswer(key).trim());
  const title = qs.length ? t("Question") : approvalTitle(e);
  const elicitation = e.text === "mcpServer/elicitation/request";
  const p = payload(e);
  const requiresForm =
    (elicitation && p.params?.requestedSchema) ||
    ([
      "agentMessage/questions",
      "item/tool/requestUserInput",
      "AskUserQuestion",
    ].includes(e.text) &&
      qs.length === 0);
  return (
    <FloatingCard
      className="approval"
      title={title}
      role="region"
      aria-label={title}
    >
      {p.params?.message && <p>{String(p.params.message)}</p>}
      <Button
        type="button"
        className="event-detail"
        onClick={() =>
          openCard({
            title: () => t("Request details"),
            content: () => <pre>{detail(e)}</pre>,
          })
        }
      >
        {t("Request details")}
      </Button>
      {qs.length > 1 && (
        <TabList
          prefix={prefix}
          panelId={panel}
          label={t("Question steps")}
          items={qs.map((q, index) => ({
            id: String(index),
            label: q.header || t("Question {number}", { number: index + 1 }),
            answered: answered(q.key),
          }))}
          value={String(activeStep)}
          choose={(value) => setStep(Number(value))}
        />
      )}
      <div
        id={panel}
        role={qs.length > 1 ? "tabpanel" : undefined}
        aria-labelledby={qs.length > 1 ? `${prefix}-${activeStep}` : undefined}
      >
        {qs.map((q, index) => (
          <fieldset
            key={q.key}
            className="question-group"
            hidden={index !== activeStep}
            disabled={busy}
          >
            <legend>{q.text}</legend>
            <div className="question-options">
              {q.options.map((o) => (
                <div key={o.label} className="question-option-card">
                  <label
                    className="question-option"
                    data-selected={(selected[q.key] ?? []).includes(o.label)}
                  >
                    <input
                      type={q.multi ? "checkbox" : "radio"}
                      name={`${e.requestId}:${q.key}`}
                      checked={(selected[q.key] ?? []).includes(o.label)}
                      onChange={(event) => {
                        if (!q.multi)
                          setOther((old) => ({ ...old, [q.key]: "" }));
                        setSelected((old) => ({
                          ...old,
                          [q.key]: q.multi
                            ? event.target.checked
                              ? [...(old[q.key] ?? []), o.label]
                              : (old[q.key] ?? []).filter((x) => x !== o.label)
                            : [o.label],
                        }));
                      }}
                    />
                    <span className="question-option-text">
                      <strong>{o.label}</strong>
                      {o.description && <small>{o.description}</small>}
                    </span>
                  </label>
                  {o.preview && (
                    <div className="question-option-preview">
                      <Markdown text={o.preview} />
                    </div>
                  )}
                </div>
              ))}
            </div>
            {q.other && (
              <div className="question-other">
                <span className="question-other-label">
                  {q.options.length ? t("Other") : t("Answer")}
                </span>
                {q.secret ? (
                  <input
                    aria-label={t(
                      q.options.length
                        ? "Other answer: {question}"
                        : "Answer: {question}",
                      {
                        question: q.text,
                      },
                    )}
                    type="password"
                    value={other[q.key] ?? ""}
                    onChange={(event) => {
                      if (!q.multi)
                        setSelected((old) => ({ ...old, [q.key]: [] }));
                      setOther((old) => ({
                        ...old,
                        [q.key]: event.target.value,
                      }));
                    }}
                  />
                ) : (
                  <div className="question-other-editor">
                    <ComposerEditor
                      ariaLabel={t(
                        q.options.length
                          ? "Other answer: {question}"
                          : "Answer: {question}",
                        {
                          question: q.text,
                        },
                      )}
                      placeholder={t("Type your answer…")}
                      value={other[q.key] ?? ""}
                      pastes={pastes}
                      onChange={(value) => {
                        if (!q.multi)
                          setSelected((old) => ({ ...old, [q.key]: [] }));
                        setOther((old) => ({ ...old, [q.key]: value }));
                      }}
                    />
                  </div>
                )}
              </div>
            )}
          </fieldset>
        ))}
      </div>
      {requiresForm && (
        <p>
          {t(
            "This request form is not supported in the web client yet. Complete it in the TUI.",
          )}
        </p>
      )}
      <div className="buttons">
        <Button
          disabled={busy || !!requiresForm || qs.some((q) => !answered(q.key))}
          onClick={() =>
            reply(
              e,
              true,
              qs.length
                ? JSON.stringify(
                    Object.fromEntries(
                      qs.map((q) => [
                        q.key,
                        {
                          selected: selected[q.key] ?? [],
                          other: otherAnswer(q.key),
                        },
                      ]),
                    ),
                  )
                : "",
            )
          }
        >
          {qs.length ? t("Submit answers") : t("Allow")}
        </Button>
        <Button disabled={busy} onClick={() => reply(e, false)}>
          {t("Deny")}
        </Button>
      </div>
    </FloatingCard>
  );
}
