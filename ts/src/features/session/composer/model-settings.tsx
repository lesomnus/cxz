import styles from "./model-settings.module.css";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { IconValueMenu } from "@lesomnus/nox";
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
          <IconValueMenu
            key={kind}
            className={`${kind}-field setting-field ${styles.field}`}
            iconClassName={`meta-label ${styles.icon}`}
            icon={
              <svg
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                {kind === "model" ? (
                  <>
                    <rect x="4" y="7" width="16" height="13" rx="3" />
                    <path d="M12 7V3M2 12v4m20-4v4M9 17h6" />
                    <circle cx="8.5" cy="12" r="1" />
                    <circle cx="15.5" cy="12" r="1" />
                  </>
                ) : (
                  <>
                    <path d="M4 18a9 9 0 1 1 16 0M12 14l4-6" />
                  </>
                )}
              </svg>
            }
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
            minMenuWidth={kind === "model" ? 180 : 140}
            disabled={disabled}
            disabledReason={
              !values.length
                ? t("Provider choices not reported")
                : t("Settings require an idle session")
            }
            choose={(choice) => change(kind, choice)}
          />
        );
      })}
    </div>
  );
}
