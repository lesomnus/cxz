import type { LoadEventDetails } from "./session-history";
import { DetailTabs, type DetailSection } from "./detail-tabs";
import { useEventDetails } from "./event-details";
import { Button } from "./button";
import { useAnchoredCard } from "./floating-card";
import { detail, payload } from "./journal";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import {
  toolLabel,
  toolOutput,
  toolState,
  toolFiles,
  fileAction,
  fileMeasure,
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
  const details = useAnchoredCard();
  const label = toolLabel(activity, agent);
  const state = toolState(activity, agent);
  const { files, omitted } = toolFiles(activity);
  const symbol = { pending: "○", working: "•", completed: "✓", failed: "×" }[
    state
  ];
  const status = (
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
  );
  return (
    <Button
      type="button"
      data-seq={seq.toString()}
      data-state={state}
      className="event-detail tool-activity detail-anchor"
      ref={details.anchor}
      data-detail-present={details.present}
      data-detail-open={details.expanded}
      aria-expanded={details.expanded}
      aria-haspopup="dialog"
      aria-controls={details.controls}
      {...details.handlers({
        title: `${label.name} · ${t("Details")}`,
        content: () => (
          <ToolDetails
            activity={activity}
            seq={seq}
            loadDetails={loadDetails}
          />
        ),
      })}
    >
      {files.length ? (
        <span className="tool-file-list">
          {files.map((file, index) => (
            <span className="tool-file-row" key={index}>
              {status}
              <span className="tool-file-summary">
                {fileAction(file)}{" "}
                <span className="tool-file-path">
                  {file.path || t("(path not reported)")}
                </span>
                {file.movePath && (
                  <>
                    {" "}
                    → <span className="tool-file-path">{file.movePath}</span>
                  </>
                )}
                {fileMeasure(file) && (
                  <span className="tool-file-measure">
                    {" "}
                    · {fileMeasure(file)}
                  </span>
                )}
              </span>
            </span>
          ))}
          {!!omitted && (
            <span className="tool-file-more">
              {t("{count} more files in Details", { count: omitted })}
            </span>
          )}
        </span>
      ) : (
        <>
          {status}
          <span className="tool-summary">
            <span>{label.name}</span>
            {label.shell && (
              <span className="tool-shell"> · {label.shell}</span>
            )}
            {label.command && (
              <span className="tool-command"> · {label.command}</span>
            )}
          </span>
        </>
      )}
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
  const sections: DetailSection[] = [
    {
      id: "input",
      label: t("Input"),
      value: activity.call
        ? detail(activity.call)
        : t("Input is not available in the loaded history."),
      language: activity.call ? "json" : "plaintext",
    },
    {
      id: "output",
      label: t("Output"),
      value:
        toolOutput(activity) ||
        (activity.result
          ? detail(activity.result)
          : t("No output recorded yet.")),
    },
  ];
  if (activity.result)
    sections.push({
      id: "result",
      label: t("Result"),
      value: detail(activity.result),
      language: "json",
    });
  for (const { request, resolution } of activity.approvals)
    sections.push({
      id: `approval-${request.seq}`,
      label: t("Approval"),
      value: JSON.stringify(
        {
          resolution:
            resolution?.text ?? t("No resolution recorded in loaded history."),
          request: payload(request),
        },
        null,
        2,
      ),
      language: "json",
    });
  return <DetailTabs sections={sections} />;
}
