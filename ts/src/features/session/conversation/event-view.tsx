import { memo } from "react";
import type { SessionEvent } from "#gen/cxz/session_pb";
import type { LoadEventDetails } from "#src/features/session/model/session-history.ts";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { ActivityCard } from "./activity-card";
import { EventDetails } from "./event-details";
import { approvalTitle } from "#src/features/session/model/journal.ts";
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
    return (
      <ActivityCard
        seq={e.seq}
        timeMs={e.timeMs}
        className={
          e.kind === "diagnostic" || e.kind === "stderr" ? "error" : ""
        }
        title={e.kind === "approval" ? approvalTitle(e) : e.kind}
        details={() => (
          <EventDetails
            event={e}
            loadDetails={e.payload.length ? undefined : loadDetails}
          />
        )}
      >
        {e.kind === "approval"
          ? approvalTitle(e)
          : `${e.kind} · ${e.text.slice(0, 160)}`}
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
