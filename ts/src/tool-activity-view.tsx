import type { LoadEventDetails } from "./session-history";
import { useEventDetails } from "./event-details";
import { Button } from "./button";
import { useFloatingCard } from "./floating-card";
import { detail } from "./journal";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import {
  toolLabel,
  toolOutput,
  toolState,
  transcriptEvents,
  type ToolActivity,
} from "./tool-activity";

export function ToolActivityView({
  activity,
  agent,
  seq,
  loadDetails,
}: {
  activity: ToolActivity;
  agent: string;
  seq: bigint;
  loadDetails?: LoadEventDetails;
}) {
  useLocale();
  const openCard = useFloatingCard();
  const label = toolLabel(activity, agent);
  const state = toolState(activity, agent);
  const symbol = { pending: "○", working: "•", completed: "✓", failed: "×" }[
    state
  ];
  return (
    <Button
      type="button"
      data-seq={seq.toString()}
      data-state={state}
      className="event-detail tool-activity"
      onClick={() =>
        openCard({
          title: `${label.name} · ${t("Details")}`,
          content: () => (
            <ToolDetails
              activity={activity}
              seq={seq}
              loadDetails={loadDetails}
            />
          ),
        })
      }
    >
      <span
        className="tool-state"
        role="img"
        aria-label={t(
          state === "completed"
            ? "Completed"
            : state === "failed"
              ? "Failed"
              : state === "working"
                ? "Working"
                : "Pending",
        )}
      >
        {symbol}
      </span>
      <span className="tool-summary">
        <span>{label.name}</span>
        {label.shell && <span className="tool-shell"> · {label.shell}</span>}
        {label.command && (
          <span className="tool-command"> · {label.command}</span>
        )}
      </span>
    </Button>
  );
}

function ToolDetails({
  activity: original,
  seq,
  loadDetails,
}: {
  activity: ToolActivity;
  seq: bigint;
  loadDetails?: LoadEventDetails;
}) {
  useLocale();
  const anchor = original.call ?? original.output[0] ?? original.result!;
  const summary = !!anchor.toolSummary;
  const state = useEventDetails(anchor, summary ? loadDetails : undefined);
  if (summary && state.error) return <p role="alert">{state.error}</p>;
  if (summary && !state.events) return <p className="muted">{t("Loading…")}</p>;
  const projected = summary
    ? transcriptEvents(state.events!).activities
    : undefined;
  const activity =
    projected?.get(seq) ??
    (projected &&
      [...projected.values()].find((activity) => {
        const event = activity.call ?? activity.output[0] ?? activity.result;
        return (
          event?.runId === anchor.runId && event.requestId === anchor.requestId
        );
      })) ??
    original;
  return (
    <>
      <h3>{t("Input")}</h3>
      {activity.call ? (
        <pre>{detail(activity.call)}</pre>
      ) : (
        <p className="muted">
          {t("Input is not available in the loaded history.")}
        </p>
      )}
      <h3>{t("Output")}</h3>
      <pre>
        {toolOutput(activity) ||
          (activity.result
            ? detail(activity.result)
            : t("No output recorded yet."))}
      </pre>
      {activity.result && (
        <>
          <h3>{t("Result")}</h3>
          <pre>{detail(activity.result)}</pre>
        </>
      )}
      {activity.approvals.map(({ request, resolution }) => (
        <div key={request.seq.toString()}>
          <h3>
            {t("Approval")} ·{" "}
            {resolution?.text ?? t("No resolution recorded in loaded history.")}
          </h3>
          <pre>{detail(request)}</pre>
        </div>
      ))}
    </>
  );
}
