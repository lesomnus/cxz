import { memo, useId } from "react";
import type { SessionEvent } from "../gen/cxz/session_pb";
import type { LoadEventDetails } from "./session-history";
import { marked } from "marked";
import DOMPurify from "dompurify";
import { useLocale } from "./i18n-react";
import { Button } from "./button";
import { EventDetails } from "./event-details";
import { approvalTitle } from "./journal";
import { AgentBrand } from "./agent-brand";
import { responseInfo } from "./response-info";
import type { ResponseCompletion } from "./response-completion";
import { ResponseFooter } from "./response-footer";
import { CopyButton } from "./copy-button";
import { EventTimePopover } from "./event-time-popover";
import { InputMessage } from "./input-message";
import { useAnchoredCard } from "./floating-card";
import type { ToolActivity } from "./tool-activity";
import { ToolActivityView } from "./tool-activity-view";

// Keep conversation link behavior scoped to this sanitizer instance.
const markdownPurifier = DOMPurify();
markdownPurifier.addHook("afterSanitizeAttributes", (node) => {
  if (node.localName === "a" && node.hasAttribute("href")) {
    node.setAttribute("target", "_blank");
    node.setAttribute("rel", "noopener noreferrer");
  }
});

function Markdown({ text }: { text: string }) {
  useLocale();
  return (
    <div
      className="markdown"
      dangerouslySetInnerHTML={{
        __html: markdownPurifier.sanitize(
          marked.parse(text, { async: false }),
          {
            FORBID_TAGS: ["img", "style", "input", "form"],
            FORBID_ATTR: ["style"],
          },
        ),
      }}
    />
  );
}
export const EventView = memo(
  function EventView({
    e,
    agent,
    completion,
    activity,
    loadDetails,
  }: {
    e: SessionEvent;
    agent: string;
    completion?: ResponseCompletion;
    activity?: ToolActivity;
    loadDetails?: LoadEventDetails;
  }) {
    useLocale();
    const details = useAnchoredCard();
    const timestamp = useId();
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
      <Button
        type="button"
        data-seq={e.seq.toString()}
        className={`event-detail detail-anchor ${e.kind === "diagnostic" || e.kind === "stderr" ? "error" : ""}`}
        ref={details.anchor}
        data-detail-present={details.present}
        data-detail-open={details.expanded}
        aria-expanded={details.expanded}
        aria-haspopup="dialog"
        aria-controls={details.controls}
        aria-describedby={timestamp}
        {...details.handlers({
          title: e.kind === "approval" ? approvalTitle(e) : e.kind,
          content: () => (
            <EventDetails
              event={e}
              loadDetails={e.payload.length ? undefined : loadDetails}
            />
          ),
        })}
      >
        <EventTimePopover timeMs={e.timeMs} id={timestamp} />
        {e.kind === "approval"
          ? approvalTitle(e)
          : `${e.kind} · ${e.text.slice(0, 160)}`}
      </Button>
    );
  },
  (previous, next) =>
    previous.e === next.e &&
    previous.agent === next.agent &&
    previous.activity === next.activity &&
    previous.loadDetails === next.loadDetails &&
    JSON.stringify(previous.completion) === JSON.stringify(next.completion),
);
