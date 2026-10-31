import { useId, useState } from "react";
import { Section, SectionGroup, SectionLayout } from "@lesomnus/nox";
import type { EventSource } from "#src/features/session/model/event-source.ts";
import {
  questions,
  payload,
  type Question,
} from "#src/features/session/model/journal.ts";
import { ResponseHeading } from "#src/features/session/conversation/response-heading.tsx";
import { PanelScroll } from "#src/shared/components/panel-scroll.tsx";
import { QuestionTabs } from "./question-tabs.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./question-history.module.css";

function answerText(q: Question, resolution: any) {
  if (!resolution) return t("Awaiting answer");
  if (resolution.state !== "allowed") return t("Canceled");
  const answer = resolution.answers?.[q.key];
  if (!answer) return t("Answer not recorded");
  if (q.secret) return t("Hidden answer");
  return [
    ...(Array.isArray(answer.selected)
      ? answer.selected.filter((value: unknown) => typeof value === "string")
      : []),
    ...(typeof answer.other === "string" && answer.other ? [answer.other] : []),
  ].join("\n\n");
}

export function QuestionHistory({
  source,
  agent,
}: {
  source: EventSource;
  agent: string;
}) {
  useLocale();
  const prefix = useId();
  const [selected, setSelected] = useState<string>();
  const event = source();
  const qs = questions(agent, event);
  const active = qs.find((q) => q.key === selected) ?? qs[0];
  const activeIndex = qs.indexOf(active);
  const resolution = payload(event).cxz_resolution;
  const state = !resolution
    ? "pending"
    : resolution.state === "allowed"
      ? "completed"
      : "failed";
  if (!active) return null;
  const panel = `${prefix}-panel`;
  return (
    <article
      className={`response question-history ${styles.card}`}
      data-seq={event.seq.toString()}
      data-state={state}
      aria-label={t("Question")}
    >
      <ResponseHeading
        agent={agent}
        response={event.response}
        timeMs={event.timeMs}
        text={qs
          .map((q) => `${q.text}\n\n${answerText(q, resolution)}`)
          .join("\n\n")}
      />
      <div className={styles.body}>
        <QuestionTabs
          prefix={prefix}
          panelId={panel}
          items={qs.map((q) => ({
            id: q.key,
            answered:
              resolution?.state === "allowed" && !!resolution.answers?.[q.key],
          }))}
          value={active.key}
          choose={setSelected}
        />
        <SectionLayout
          id={panel}
          role="tabpanel"
          aria-labelledby={`${prefix}-${activeIndex}`}
          className={styles.panel}
          align="start"
          maxWidth="none"
        >
          <SectionGroup className={styles.group}>
            <Section
              className={styles.question}
              role="group"
              aria-label={t("Question")}
            >
              <PanelScroll
                key={`question-${active.key}`}
                scrollbarLabel={t("Question")}
              >
                <Markdown text={active.text} />
              </PanelScroll>
            </Section>
            <Section
              className={styles.answer}
              role="group"
              aria-label={t("Answer")}
            >
              <PanelScroll
                key={`answer-${active.key}`}
                scrollbarLabel={t("Answer")}
              >
                <Markdown text={answerText(active, resolution)} />
              </PanelScroll>
            </Section>
          </SectionGroup>
        </SectionLayout>
      </div>
    </article>
  );
}
