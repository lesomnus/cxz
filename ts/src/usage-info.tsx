import { t, currentLocale } from "./i18n";
import { useLocale } from "./i18n-react";
import {
  formatTokens,
  formatReset,
  quotaDots,
  type SessionInfo,
} from "./session-info";

export function UsageInfo({ info }: { info: SessionInfo }) {
  useLocale();
  const known = info.contextUsed !== undefined && !!info.contextWindow;
  const ratio = known
    ? Math.min(1, info.contextUsed! / info.contextWindow!)
    : 0;
  return (
    <div className="usage-info">
      <span
        className="meta-popover"
        tabIndex={0}
        aria-describedby="quota-popover"
      >
        <span
          className="quota-dots"
          role="img"
          aria-label={
            info.remaining === undefined
              ? t("Remaining quota not reported")
              : t("Remaining quota {percent}%", {
                  percent: Math.round(info.remaining),
                })
          }
        >
          {quotaDots(info.remaining)}
        </span>
        <span id="quota-popover" className="meta-tooltip" role="tooltip">
          {info.quotaLabel || t("Usage")} ·{" "}
          {info.remaining === undefined
            ? "—"
            : t("{percent}% remaining", {
                percent: Math.round(info.remaining),
              })}
          <br />
          {t("Reset")} {formatReset(info.reset)}
          {info.reset
            ? ` · ${new Date(info.reset).toLocaleDateString(currentLocale())}`
            : ""}
        </span>
      </span>
      <span
        className="meta-popover context-popover"
        tabIndex={0}
        aria-describedby="context-popover"
      >
        <svg
          className="context-donut"
          width="18"
          height="18"
          viewBox="0 0 20 20"
          role="img"
          aria-label={t("Context usage")}
        >
          <circle
            className="context-track"
            cx="10"
            cy="10"
            r="7"
            fill="none"
            strokeWidth="3"
          />
          <circle
            className="context-fill"
            cx="10"
            cy="10"
            r="7"
            fill="none"
            stroke="currentColor"
            strokeWidth="3"
            pathLength="1"
            strokeDasharray={`${ratio} 1`}
            transform="rotate(-90 10 10)"
          />
        </svg>
        <span id="context-popover" className="meta-tooltip" role="tooltip">
          {formatTokens(info.contextUsed)}/{formatTokens(info.contextWindow)}
        </span>
      </span>
    </div>
  );
}
