import { memo } from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import type { LoadEventDetails } from "#src/features/session/model/session-history.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { ActivityCard } from "./activity-card";
import { EventDetails } from "./event-details";
import {
  approvalTitle,
  isQuestionEvent,
  questions,
} from "#src/features/session/model/journal.ts";
import { QuestionHistory } from "#src/features/session/questions/question-history.tsx";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { responseInfo } from "#src/features/session/model/response-info.ts";
import type { ResponseCompletion } from "#src/features/session/model/response-completion.ts";
import type { TurnSummary } from "#src/features/session/model/session-aux.ts";
import { ResponseFooter } from "./response-footer";
import { TurnSummaryView } from "./turn-summary";
import { CopyButton } from "#src/shared/components/copy-button.tsx";
import { InputMessage } from "./input-message";
import type { ToolActivity } from "#src/features/session/model/tool-activity.ts";
import { ToolActivityView } from "./tool-activity-view";
import { eventDiagnostic } from "#src/features/session/model/event-diagnostic.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./activity-card.module.css";

export const EventView = memo(
  function EventView({
    e,
    agent,
    completion,
    summary,
    activity,
    loadDetails,
  }: {
    e: SessionEvent;
    agent: string;
    completion?: ResponseCompletion;
    summary?: TurnSummary;
    activity?: ToolActivity;
    loadDetails?: LoadEventDetails;
  }) {
    useLocale();
    if (isQuestionEvent(e) && questions(agent, e).length)
      return <QuestionHistory event={e} agent={agent} />;
    if (activity)
      return (
        <ToolActivityView
          activity={activity}
          agent={agent}
          seq={e.seq}
          timeMs={e.timeMs}
          loadDetails={loadDetails}
        />
      );
    if (e.kind === "assistant") {
      const info = responseInfo(e.response);
      return (
        <article className="response" data-seq={e.seq.toString()}>
          <small className="response-heading">
            <AgentBrand agent={agent} />
            {info.label && (
              <span className="response-settings" title={info.description}>
                {info.label}
              </span>
            )}
            <CopyButton value={e.text} className="copy-control" />
          </small>
          <Markdown text={e.text} />
          <ResponseFooter timeMs={e.timeMs} completion={completion} />
          {summary && <TurnSummaryView summary={summary} />}
        </article>
      );
    }
    if (e.kind === "input")
      return (
        <article className="input" data-seq={e.seq.toString()}>
          <InputMessage event={e} />
        </article>
      );
    const diagnostic = eventDiagnostic(e);
    return (
      <ActivityCard
        seq={e.seq}
        timeMs={e.timeMs}
        tone={diagnostic ? "error" : undefined}
        title={
          diagnostic
            ? t("Error")
            : e.kind === "approval"
              ? approvalTitle(e)
              : e.kind
        }
        details={() => (
          <EventDetails
            event={e}
            loadDetails={
              diagnostic
                ? loadDetails
                : e.payload.length
                  ? undefined
                  : loadDetails
            }
          />
        )}
      >
        {diagnostic ? (
          <span className={styles.diagnostic}>
            <span>
              {t("Error")} · {diagnostic.message || t("Request failed")}
            </span>
            {(diagnostic.code || diagnostic.retrying !== undefined) && (
              <small className={styles.metadata}>
                {[
                  diagnostic.code,
                  diagnostic.retrying === true
                    ? t("Retrying…")
                    : diagnostic.retrying === false
                      ? t("No automatic retry")
                      : undefined,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </small>
            )}
          </span>
        ) : e.kind === "approval" ? (
          approvalTitle(e)
        ) : (
          `${e.kind} · ${e.text.slice(0, 160)}`
        )}
      </ActivityCard>
    );
  },
  (previous, next) =>
    previous.e === next.e &&
    previous.agent === next.agent &&
    previous.activity === next.activity &&
    previous.loadDetails === next.loadDetails &&
    previous.summary?.text === next.summary?.text &&
    previous.summary?.loading === next.summary?.loading &&
    previous.summary?.failed === next.summary?.failed &&
    JSON.stringify(previous.completion) === JSON.stringify(next.completion),
);
