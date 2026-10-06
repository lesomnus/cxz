import type { ResponseMetadata } from "../gen/cxz/session_pb";

export function responseInfo(response?: ResponseMetadata) {
  const fields = [
    ["Model", response?.model, response?.modelSource],
    ["Effort", response?.effort, response?.effortSource],
  ].filter(([, value]) => value);
  return {
    label: fields.map(([, value]) => value).join(" · "),
    description: fields
      .map(([name, value, source]) => {
        const origin =
          source === "response"
            ? "응답 보고값"
            : source === "settings"
              ? "적용된 설정"
              : source === "requested"
                ? "요청한 설정"
                : "저장된 값";
        return `${name}: ${value} (${origin})`;
      })
      .join("\n"),
  };
}
