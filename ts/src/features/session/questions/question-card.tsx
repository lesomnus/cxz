import { Icon } from "#src/shared/icons/icon.tsx";
import { useId, useLayoutEffect, useRef, useState } from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  Button,
  SlidingPages,
  SlidingPage,
  dialogEntranceClassName,
} from "@lesomnus/nox";
import { ConfirmButton } from "@lesomnus/nox";
import {
  FloatingCard,
  useFloatingCard,
  useQuestionExpansion,
} from "#src/features/session/cards/floating-card.tsx";
import {
  approvalTitle,
  questions,
  payload,
  detail,
} from "#src/features/session/model/journal.ts";
import { ComposerEditor } from "#src/features/session/composer/composer-editor.tsx";
import {
  attachmentsReady,
  type ComposerPaste,
} from "#src/features/session/composer/composer-pastes.ts";
import type { UploadFile } from "#src/features/session/composer/composer-upload.ts";
import type { DownloadAttachment } from "#src/features/session/composer/composer-download.ts";
import type { SecretFiles } from "#src/features/session/composer/secret-paste.ts";
import { composerPrompt } from "#src/features/session/composer/composer-code.ts";
import { QuestionTabs } from "./question-tabs.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { ScrollFade } from "#src/shared/components/scroll-fade.tsx";
import {
  emptyQuestionDraft,
  type QuestionDrafts,
} from "#src/features/session/model/question-drafts.ts";

type QuestionCardProps = {
  e: SessionEvent;
  agent: string;
  pastes: Map<string, ComposerPaste>;
  upload?: UploadFile;
  secretFiles?: SecretFiles;
  downloadAttachment?: DownloadAttachment;
  busy: boolean;
  reply: (e: SessionEvent, allow: boolean, answers?: string) => Promise<void>;
  drafts?: QuestionDrafts;
  session?: string;
};

export function QuestionCard(props: QuestionCardProps) {
  return (
    <QuestionCardContent
      key={`${props.session ?? ""}:${props.agent}:${props.e.runId}:${props.e.requestId}`}
      {...props}
    />
  );
}

function QuestionCardContent({
  e,
  agent,
  pastes,
  upload,
  secretFiles,
  downloadAttachment,
  busy,
  reply,
  drafts,
  session,
}: QuestionCardProps) {
  useLocale();
  const openCard = useFloatingCard();
  const qs = questions(agent, e);
  const [initial] = useState(() =>
    drafts && session && qs.length
      ? drafts.get(session, e, qs)
      : emptyQuestionDraft(),
  );
  const [selected, setSelected] = useState(initial.selected);
  const [other, setOther] = useState(initial.other);
  const [step, setStep] = useState(initial.step);
  useLayoutEffect(() => {
    if (drafts && session && qs.length)
      drafts.set(session, e, qs, { selected, other, step });
  }, [drafts, session, e.runId, e.requestId, qs, selected, other, step]);
  const [, refreshUploads] = useState(0);
  const submit = useRef<HTMLButtonElement>(null);
  const next = useRef<HTMLButtonElement>(null);
  const answerShortcut = () => {
    if (busy || !ready || !qs[activeStep] || !answered(qs[activeStep].key))
      return;
    if (!submit.current?.disabled)
      submit.current?.focus({ preventScroll: true });
    else if (!next.current?.disabled) next.current?.click();
  };
  const prefix = useId();
  const { expanded, toggle } = useQuestionExpansion(prefix);
  const activeStep = Math.min(step, qs.length - 1);
  const panel = `${prefix}-panel`;
  const otherAnswer = (key: string) => composerPrompt(other[key] ?? "", pastes);
  const answered = (key: string) =>
    !!(selected[key]?.length || otherAnswer(key).trim());
  const ready = qs.every((q) => attachmentsReady(other[q.key] ?? "", pastes));
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
      className={`${dialogEntranceClassName} approval ${qs.length ? "question-card" : ""}`}
      data-enter-effect="fade"
      hideHeading={qs.length > 0}
      title={title}
      role="region"
      aria-label={title}
      data-expanded={expanded}
    >
      {!qs.length && p.params?.message && <p>{String(p.params.message)}</p>}
      {!qs.length && (
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
      )}
      {qs.length > 0 && (
        <header className="question-header">
          <QuestionTabs
            prefix={prefix}
            panelId={panel}
            items={qs.map((q, index) => ({
              id: String(index),
              answered: answered(q.key),
            }))}
            value={String(activeStep)}
            choose={(value) => setStep(Number(value))}
          />
          <Button
            type="button"
            className="question-size-toggle"
            aria-label={t(expanded ? "Collapse question" : "Expand question")}
            title={t(expanded ? "Collapse question" : "Expand question")}
            aria-expanded={expanded}
            aria-controls={panel}
            onClick={toggle}
          >
            <Icon
              name={expanded ? "collapse" : "expand"}
              width="16"
              height="16"
              aria-hidden="true"
            />
          </Button>
        </header>
      )}
      <SlidingPages
        value={qs[activeStep]?.key ?? ""}
        resize="none"
        className={`question-panels ${qs.length ? "question-body" : ""}`}
        id={panel}
        role={qs.length ? "tabpanel" : undefined}
        aria-labelledby={qs.length ? `${prefix}-${activeStep}` : undefined}
      >
        {qs.map((q, index) => (
          <SlidingPage key={q.key} value={q.key} className="question-page">
            <fieldset
              className="question-group"
              aria-labelledby={`${prefix}-question-${index}`}
              data-active={index === activeStep}
              disabled={busy}
            >
              <div className="question-text" id={`${prefix}-question-${index}`}>
                <Markdown text={q.text} />
              </div>
              <ScrollFade viewportClassName="question-options">
                {q.options.map((o) => (
                  <div
                    key={o.label}
                    className="question-option-card"
                    data-selected={(selected[q.key] ?? []).includes(o.label)}
                  >
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
                                : (old[q.key] ?? []).filter(
                                    (x) => x !== o.label,
                                  )
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
              </ScrollFade>
              {q.other && (
                <div
                  className="question-other"
                  data-selected={!!otherAnswer(q.key).trim()}
                >
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
                      onKeyDown={(event) => {
                        if (
                          event.ctrlKey &&
                          event.key === "Enter" &&
                          !event.nativeEvent.isComposing
                        ) {
                          event.preventDefault();
                          event.stopPropagation();
                          if (!event.repeat) answerShortcut();
                        }
                      }}
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
                        readOnly={busy}
                        upload={upload}
                        secretFiles={secretFiles}
                        downloadAttachment={downloadAttachment}
                        detectSecrets
                        onUploadChange={() => {
                          if (drafts && session)
                            drafts.persistAttachments(session, e, q.key);
                          refreshUploads((value) => value + 1);
                        }}
                        onSubmitShortcut={answerShortcut}
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
          </SlidingPage>
        ))}
      </SlidingPages>
      {requiresForm && (
        <p>
          {t(
            "This request form is not supported in the web client yet. Complete it in the TUI.",
          )}
        </p>
      )}
      <div className={`buttons ${qs.length ? "question-footer" : ""}`}>
        {qs.length ? (
          <ConfirmButton
            disabled={busy}
            confirmationLabel={t("Confirm cancel")}
            onConfirm={() => reply(e, false)}
          >
            {t("Cancel")}
          </ConfirmButton>
        ) : (
          <Button disabled={busy} onClick={() => reply(e, false)}>
            {t("Deny")}
          </Button>
        )}
        <Button
          ref={submit}
          disabled={
            busy || !ready || !!requiresForm || qs.some((q) => !answered(q.key))
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
          {qs.length ? t("Submit") : t("Allow")}
        </Button>
        {!!qs.length && (
          <Button
            ref={next}
            disabled={busy || activeStep >= qs.length - 1}
            onClick={() => setStep((old) => Math.min(old + 1, qs.length - 1))}
          >
            {t("Next")}
          </Button>
        )}
      </div>
    </FloatingCard>
  );
}
