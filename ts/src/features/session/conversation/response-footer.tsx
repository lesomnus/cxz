import { Icon, type IconName } from "#src/shared/icons/icon.tsx";
import { t, translateKnown, currentLocale } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  durationLabel,
  type ResponseCompletion,
} from "#src/features/session/model/response-completion.ts";
import { formatTokens } from "#src/features/session/model/session-info.ts";
import styles from "./response-footer.module.css";
import { CompactNumber, ElapsedTime } from "@lesomnus/nox";

const names: Record<string, string> = {
  input_tokens: "Input",
  output_tokens: "Output",
  cache_read_tokens: "Cache read",
  cache_write_tokens: "Cache write",
  reasoning_tokens: "Reasoning",
  total_tokens: "Total",
  cost_usd: "Cost",
  api_duration_ms: "API time",
  api_turns: "API turns",
  tool_calls: "Tools",
};

const icons: Record<string, IconName> = {
  duration: "duration",
  input_tokens: "tokens-input",
  output_tokens: "tokens-output",
  cache_read_tokens: "tokens-cache-read",
  cache_write_tokens: "tokens-cache-write",
  reasoning_tokens: "tokens-reasoning",
  total_tokens: "tokens-total",
  cost_usd: "cost",
  api_duration_ms: "api-duration",
  api_turns: "api-turns",
  tool_calls: "tool-calls",
};

function MetricIcon({ kind }: { kind: string }) {
  useLocale();
  return <Icon name={icons[kind] ?? "unknown"} size={12} />;
}

function metricValue(key: string, value: number) {
  return key === "cost_usd"
    ? `$${value}`
    : key === "api_duration_ms"
      ? durationLabel(value)
      : key.includes("tokens")
        ? formatTokens(value)
        : value.toLocaleString(currentLocale());
}

function metricTitle(key: string, value: number, scope?: string) {
  if (key === "api_duration_ms")
    return t("API time reported by the agent: {duration} ({milliseconds} ms)", {
      duration: durationLabel(value),
      milliseconds: value.toLocaleString(currentLocale()),
    });
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
  completion,
}: {
  completion?: ResponseCompletion;
}) {
  useLocale();
  if (
    !completion ||
    (completion.durationMs === undefined &&
      !Object.keys(completion.metrics).some((key) => names[key]))
  )
    return null;
  return (
    <footer className="response-footer">
      {completion && (
        <div className="response-metrics" aria-label={t("Response metrics")}>
          {completion.durationMs !== undefined && (
            <span
              className={`response-metric ${styles.metric}`}
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
              <span className={styles.value}>
                <ElapsedTime
                  durationMs={completion.durationMs}
                  label={t("Duration: {duration}", {
                    duration: durationLabel(completion.durationMs),
                  })}
                />
              </span>
              <MetricIcon kind="duration" />
            </span>
          )}
          {Object.entries(completion.metrics).map(
            ([key, value]) =>
              names[key] && (
                <span
                  key={key}
                  className={`response-metric ${styles.metric}`}
                  data-metric={key}
                  role="img"
                  aria-label={metricTitle(key, value, completion.tokenScope)}
                  title={metricTitle(key, value, completion.tokenScope)}
                >
                  <span className={styles.value}>
                    {key === "api_duration_ms" ? (
                      <ElapsedTime
                        durationMs={value}
                        label={metricTitle(key, value)}
                      />
                    ) : (
                      <CompactNumber
                        value={value}
                        mode={
                          key === "tool_calls" || key === "api_turns"
                            ? "integer"
                            : "compact"
                        }
                        label={metricTitle(key, value, completion.tokenScope)}
                      />
                    )}
                  </span>
                  <MetricIcon kind={key} />
                </span>
              ),
          )}
        </div>
      )}
    </footer>
  );
}
