import { t, translateKnown, currentLocale } from "./i18n";
import { useLocale } from "./i18n-react";
import { messageDate } from "./message-time";
import { durationLabel, type ResponseCompletion } from "./response-completion";
import { formatTokens } from "./session-info";

const names: Record<string, string> = {
  input_tokens: "Input",
  output_tokens: "Output",
  cache_read_tokens: "Cache read",
  cache_write_tokens: "Cache write",
  reasoning_tokens: "Reasoning",
  total_tokens: "Total",
  cost_usd: "Cost",
  api_duration_ms: "API",
  api_turns: "API turns",
  tool_calls: "Tools",
};

const icons: Record<string, string> = {
  duration:
    "M9 2h6M12 2v3M17 5l2-2M12 9v4l3 2 M20 13a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
  input_tokens: "M20 4v16M4 12h12M11 7l5 5-5 5",
  output_tokens: "M4 4v16M20 12H8M13 7l-5 5 5 5",
  cache_read_tokens:
    "M4 7V5c0-4 16-4 16 0v2M4 5c0 4 16 4 16 0M4 11v8c0 4 16 4 16 0v-8M4 19c0 4 16 4 16 0M3 12h10M9 9l4 3-4 3",
  cache_write_tokens:
    "M4 7V5c0-4 16-4 16 0v2M4 5c0 4 16 4 16 0M4 11v8c0 4 16 4 16 0v-8M4 19c0 4 16 4 16 0M21 12H11M15 9l-4 3 4 3",
  reasoning_tokens: "M12 3l2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5Z",
  total_tokens: "M19 4H5l7 8-7 8h14",
  cost_usd: "M12 2v20M18 6c-2-3-12-3-12 2 0 6 12 2 12 8 0 5-10 5-12 2",
  api_duration_ms: "M4 19a10 10 0 1 1 16 0M12 12l5-5M3 12h2M19 12h2M12 2v2",
  api_turns: "M20 7H5l4-4M4 17h15l-4 4M20 7v5M4 17v-5",
  tool_calls:
    "M14 6a5 5 0 0 0-6 6L3 17a3 3 0 0 0 4 4l5-5a5 5 0 0 0 6-6l-3 3-4-4Z",
};

function MetricIcon({ kind }: { kind: string }) {
  useLocale();
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={icons[kind]} />
    </svg>
  );
}

function metricValue(key: string, value: number) {
  return key === "cost_usd"
    ? `$${value.toFixed(4)}`
    : key === "api_duration_ms"
      ? durationLabel(value)
      : key.includes("tokens")
        ? formatTokens(value)
        : value.toLocaleString(currentLocale());
}

function metricTitle(key: string, value: number, scope?: string) {
  return key.includes("tokens")
    ? t("{name} — {scope}: {count} tokens{included}", {
        name: translateKnown(names[key]),
        scope: scope === "last_call" ? t("Last model call") : t("Entire turn"),
        count: value.toLocaleString(currentLocale()),
        included:
          key === "reasoning_tokens"
            ? t(" (Included in output tokens)")
            : key === "cache_read_tokens" && scope === "last_call"
              ? t(" (Included in input tokens)")
              : "",
      })
    : t("{name}: {value}", {
        name: translateKnown(names[key]),
        value: metricValue(key, value),
      });
}

export function ResponseFooter({
  timeMs,
  completion,
}: {
  timeMs: bigint;
  completion?: ResponseCompletion;
}) {
  useLocale();
  if (!completion) return null;
  const date = messageDate(timeMs);
  return (
    <footer className="response-footer">
      {completion && (
        <div className="response-metrics" aria-label={t("Response metrics")}>
          {date && (
            <time
              dateTime={date.toISOString()}
              title={t("Time cxz received and recorded the response")}
            >
              {date.toLocaleString(currentLocale(), {
                month: "2-digit",
                day: "2-digit",
                hour: "2-digit",
                minute: "2-digit",
                second: "2-digit",
                hour12: false,
              })}
            </time>
          )}
          {completion.durationMs !== undefined && (
            <span
              className="response-metric"
              data-metric="duration"
              role="img"
              aria-label={t("Duration: {duration}", {
                duration: durationLabel(completion.durationMs),
              })}
              title={
                completion.durationSource === "provider"
                  ? t("Turn duration reported by the agent")
                  : t(
                      "Time measured by cxz from input to turn completion, including tool and approval waits",
                    )
              }
            >
              <MetricIcon kind="duration" />
              {durationLabel(completion.durationMs)}
            </span>
          )}
          {Object.entries(completion.metrics).map(
            ([key, value]) =>
              names[key] && (
                <span
                  key={key}
                  className="response-metric"
                  data-metric={key}
                  role="img"
                  aria-label={metricTitle(key, value, completion.tokenScope)}
                  title={metricTitle(key, value, completion.tokenScope)}
                >
                  <MetricIcon kind={key} />
                  {metricValue(key, value)}
                </span>
              ),
          )}
        </div>
      )}
    </footer>
  );
}
