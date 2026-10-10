import { t } from "#src/shared/i18n/i18n.ts";
import type { ResponseMetadata } from "#gen/cxz/session_pb";

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
            ? "Reported by response"
            : source === "settings"
              ? "Applied setting"
              : source === "requested"
                ? "Requested setting"
                : "Stored value";
        return t(
          name === "Model"
            ? "Model: {value} ({source})"
            : "Effort: {value} ({source})",
          { value: value!, source: t(origin) },
        );
      })
      .join("\n"),
  };
}
