import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { ValueMenu } from "@lesomnus/cxz-ui";
import type { Session, SessionEvent } from "#gen/cxz/session_pb";
import { payload } from "#src/features/session/model/journal.ts";
import type { SessionInfo } from "#src/features/session/model/session-info.ts";

type ModelOption = {
  id: string;
  resolved_id?: string;
  name?: string;
  efforts?: string[];
  default_effort?: string;
  default?: boolean;
};
export type ModelCatalog = {
  models: ModelOption[];
  model?: string;
  effort?: string;
  effective_model?: string;
};
export function modelCatalog(
  s: Session | undefined,
  events: SessionEvent[],
): ModelCatalog | undefined {
  const event = [...events]
    .reverse()
    .find((e) => e.kind === "models" && e.runId === s?.status?.runId);
  if (!event) return;
  const p = payload(event);
  if (!Array.isArray(p.models)) return;
  return {
    ...p,
    models: p.models.filter(
      (m: ModelOption) => m && typeof m.id === "string" && /^\S+$/.test(m.id),
    ),
  };
}
export function selectedModel(c: ModelCatalog) {
  const applied = c.effective_model || c.model;
  const exact = c.models.find(
    (m) =>
      m.id === c.model &&
      (!applied || m.id === applied || m.resolved_id === applied),
  );
  if (exact) return exact;
  if (applied && applied !== "default")
    return (
      c.models.find((m) => m.id === applied || m.resolved_id === applied) ||
      c.models.find((m) => m.id === c.model && !m.resolved_id)
    );
  return c.models.find((m) => m.default || m.id === "default");
}
export function ModelSettings({
  session,
  info,
  catalog,
  busy,
  change,
}: {
  session?: Session;
  info: SessionInfo;
  catalog?: ModelCatalog;
  busy: boolean;
  change: (kind: "model" | "effort", value: string) => void;
}) {
  useLocale();
  const selected = catalog && selectedModel(catalog);
  const models = catalog?.models.map((m) => m.id) ?? [];
  if (catalog?.models.some((m) => m.default) && !models.includes("default"))
    models.unshift("default");
  const efforts = (selected?.efforts || []).filter(
    (v) => typeof v === "string" && /^\S+$/.test(v),
  );
  if (
    efforts.length &&
    (session?.agent === "claude" || selected?.default_effort)
  )
    efforts.push("default");
  return (
    <div className="model-info">
      {(["model", "effort"] as const).map((kind) => {
        const label = kind === "model" ? t("Model") : t("Effort");
        const values = [...new Set(kind === "model" ? models : efforts)];
        const value =
          (kind === "model" ? catalog?.model : catalog?.effort) || "default";
        const disabled =
          busy || session?.status?.state !== "idle" || !values.length;
        return (
          <div key={kind} className={`${kind}-field setting-field`}>
            <span className="meta-label">{label}</span>
            <ValueMenu
              variant="compact"
              label={label}
              display={info[kind] || "—"}
              value={value}
              options={values.map((value) => ({
                value,
                label: value,
                muted: value === "default",
              }))}
              muted={value === "default"}
              minMenuWidth={kind === "model" ? 240 : 140}
              disabled={disabled}
              disabledReason={
                !values.length
                  ? t("Provider choices not reported")
                  : t("Settings require an idle session")
              }
              choose={(choice) => change(kind, choice)}
            />
          </div>
        );
      })}
    </div>
  );
}
