import type { SessionEvent } from "#gen/cxz/session_pb";
import {
  questions,
  payload,
  type Question,
} from "#src/features/session/model/journal.ts";
import { ActivityCard } from "#src/features/session/conversation/activity-card.tsx";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
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
  event,
  agent,
}: {
  event: SessionEvent;
  agent: string;
}) {
  const qs = questions(agent, event);
  const resolution = payload(event).cxz_resolution;
  const state = !resolution
    ? "pending"
    : resolution.state === "allowed"
      ? "completed"
      : "failed";
  return (
    <ActivityCard
      seq={event.seq}
      timeMs={event.timeMs}
      title={t("Question")}
      className="question-history"
      state={state}
      details={() => (
        <div className={styles.details}>
          {qs.map((q, index) => (
            <section key={q.key}>
              <div className={styles.question}>
                <small>Q{index + 1}</small>
                <Markdown text={q.text} />
              </div>
              <div className={styles.answer}>
                <small>{t("Answer")}</small>
                <Markdown text={answerText(q, resolution)} />
              </div>
            </section>
          ))}
        </div>
      )}
    >
      {state === "pending" ? "◯" : state === "completed" ? "✓" : "×"}{" "}
      {t("Question")} ·{" "}
      {qs.map((q) => `${q.text} → ${answerText(q, resolution)}`).join(" · ")}
    </ActivityCard>
  );
}
