import { useState } from "react";
import type { SessionEvent } from "../gen/cxz/session_pb";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import { Button } from "./button";
import { FloatingCard, useFloatingCard } from "./floating-card";
import { approvalTitle, questions, payload, detail } from "./journal";
import { ComposerEditor } from "./composer-editor";
import type { ComposerPaste } from "./composer-pastes";
import { composerPrompt } from "./composer-code";

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
  const otherAnswer = (key: string) => composerPrompt(other[key] ?? "", pastes);
  const elicitation = e.text === "mcpServer/elicitation/request";
  const p = payload(e);
  const requiresForm =
    (elicitation && p.params?.requestedSchema) ||
    e.text === "agentMessage/questions";
  return (
    <FloatingCard
      className="approval"
      title={approvalTitle(e)}
      role="region"
      aria-label={approvalTitle(e)}
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
      {qs.map((q) => (
        <fieldset key={q.key} className="question-group">
          <legend>{q.text}</legend>
          <div className="question-options">
            {q.options.map((o) => (
              <label
                key={o.label}
                className="question-option"
                data-selected={(selected[q.key] ?? []).includes(o.label)}
              >
                <input
                  type={q.multi ? "checkbox" : "radio"}
                  name={`${e.requestId}:${q.key}`}
                  checked={(selected[q.key] ?? []).includes(o.label)}
                  onChange={(event) =>
                    setSelected((old) => ({
                      ...old,
                      [q.key]: q.multi
                        ? event.target.checked
                          ? [...(old[q.key] ?? []), o.label]
                          : (old[q.key] ?? []).filter((x) => x !== o.label)
                        : [o.label],
                    }))
                  }
                />
                <span className="question-option-text">
                  <strong>{o.label}</strong>
                  {o.description && <small>{o.description}</small>}
                </span>
              </label>
            ))}
          </div>
          {q.other && (
            <div className="question-other">
              <span className="question-other-label">{t("Other")}</span>
              {q.secret ? (
                <input
                  aria-label={t("Other answer: {question}", {
                    question: q.text,
                  })}
                  type="password"
                  value={other[q.key] ?? ""}
                  onChange={(event) =>
                    setOther((old) => ({ ...old, [q.key]: event.target.value }))
                  }
                />
              ) : (
                <div className="question-other-editor">
                  <ComposerEditor
                    ariaLabel={t("Other answer: {question}", {
                      question: q.text,
                    })}
                    placeholder={t("Type your answer…")}
                    value={other[q.key] ?? ""}
                    pastes={pastes}
                    onChange={(value) =>
                      setOther((old) => ({ ...old, [q.key]: value }))
                    }
                  />
                </div>
              )}
            </div>
          )}
        </fieldset>
      ))}
      {requiresForm && (
        <p>
          {t(
            "This request form is not supported in the web client yet. Complete it in the TUI.",
          )}
        </p>
      )}
      <div className="buttons">
        <Button
          disabled={
            busy ||
            !!requiresForm ||
            qs.some(
              (q) => !(selected[q.key]?.length || otherAnswer(q.key).trim()),
            )
          }
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
