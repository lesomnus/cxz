import type { ResponseMetadata } from "#gen/cxz/session_pb";
import { AgentBrand } from "#src/shared/components/agent-brand.tsx";
import { CopyButton } from "#src/shared/components/copy-button.tsx";
import { responseInfo } from "#src/features/session/model/response-info.ts";
import { t, currentLocale } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { useMessageTime } from "./use-message-time";
import styles from "./response-heading.module.css";

export function ResponseHeading({
  agent,
  response,
  timeMs,
  text,
}: {
  agent: string;
  response?: ResponseMetadata;
  timeMs: bigint;
  text: string;
}) {
  useLocale();
  const info = responseInfo(response);
  const { date, relative } = useMessageTime(timeMs);
  return (
    <small className={`response-heading ${styles.heading}`}>
      <AgentBrand agent={agent} />
      <span className={styles.metadata}>
        {info.label && (
          <span
            className={`response-settings ${styles.settings}`}
            title={info.description}
          >
            {info.label}
          </span>
        )}
        {date && (
          <span className={`response-time ${styles.time}`}>
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
            {relative && (
              <span className="response-relative-time"> - {relative}</span>
            )}
          </span>
        )}
      </span>
      <CopyButton value={text} className="copy-control" />
    </small>
  );
}
