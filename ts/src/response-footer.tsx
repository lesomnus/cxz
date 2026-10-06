import { Button } from "./button";
import { messageDate } from "./message-time";
import { durationLabel, type ResponseCompletion } from "./response-completion";
import { formatTokens } from "./session-info";

const names: Record<string, string> = {
  input_tokens: "입력",
  output_tokens: "출력",
  cache_read_tokens: "캐시 읽기",
  cache_write_tokens: "캐시 쓰기",
  reasoning_tokens: "추론",
  total_tokens: "전체",
  cost_usd: "비용",
  api_duration_ms: "API",
  api_turns: "API 턴",
  tool_calls: "도구",
};

export function ResponseFooter({
  seq,
  timeMs,
  text,
  completion,
}: {
  seq: bigint;
  timeMs: bigint;
  text: string;
  completion?: ResponseCompletion;
}) {
  const date = completion && messageDate(timeMs);
  const tooltip = `copy-${seq}`;
  return (
    <footer className="response-footer">
      {completion && (
        <div className="response-metrics" aria-label="Response metrics">
          {date && (
            <time
              dateTime={date.toISOString()}
              title="cxz가 응답을 수신해 기록한 시각"
            >
              {date.toLocaleString("ko-KR", {
                year: "numeric",
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
              title={
                completion.durationSource === "provider"
                  ? "에이전트가 보고한 턴 소요 시간"
                  : "cxz가 입력 전송부터 턴 종료까지 측정한 시간 (도구·승인 대기 포함)"
              }
            >
              {durationLabel(completion.durationMs)}
            </span>
          )}
          {Object.entries(completion.metrics).map(
            ([key, value]) =>
              names[key] && (
                <span
                  key={key}
                  title={
                    key.includes("tokens")
                      ? `${completion.tokenScope === "last_call" ? "마지막 모델 호출" : "턴 전체"}: ${value.toLocaleString()} tokens${key === "reasoning_tokens" ? " (출력 토큰에 포함)" : key === "cache_read_tokens" && completion.tokenScope === "last_call" ? " (입력 토큰에 포함)" : ""}`
                      : names[key]
                  }
                >
                  {completion.tokenScope === "last_call" &&
                  key.includes("tokens")
                    ? "호출 "
                    : ""}
                  {names[key]}{" "}
                  {key === "cost_usd"
                    ? `$${value.toFixed(4)}`
                    : key === "api_duration_ms"
                      ? durationLabel(value)
                      : key.includes("tokens")
                        ? formatTokens(value)
                        : value.toLocaleString()}
                </span>
              ),
          )}
        </div>
      )}
      <span className="meta-popover copy-control">
        <Button
          className="copy"
          aria-label="Copy"
          aria-describedby={tooltip}
          onClick={() => navigator.clipboard.writeText(text).catch(() => {})}
        >
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <rect x="8" y="8" width="12" height="12" rx="2" />
            <path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3" />
          </svg>
        </Button>
        <span id={tooltip} className="meta-tooltip" role="tooltip">
          copy
        </span>
      </span>
    </footer>
  );
}
