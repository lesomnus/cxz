import { Form, FormField, SegmentedControl } from "@lesomnus/nox";
import { lazy, Suspense, useId, useState } from "react";
import { isAlias } from "yaml";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import type { readKubernetesProjectSettings } from "./kubernetes-project-settings";
import styles from "./project-add.module.css";

const YamlEditor = lazy(() => import("./kubernetes-project-yaml"));
const fields = [
  { label: "Container image", path: ["image"], fullWidth: true },
  { label: "Container user", path: ["user"] },
  { label: "Storage capacity", path: ["storage"], placeholder: "20Gi" },
  { label: "CPU request", path: ["requests", "cpu"], placeholder: "500m" },
  { label: "Memory request", path: ["requests", "memory"], placeholder: "2Gi" },
  { label: "CPU limit", path: ["limits", "cpu"], placeholder: "1" },
  { label: "Memory limit", path: ["limits", "memory"], placeholder: "4Gi" },
  {
    label: "Dockerfile path",
    path: ["build", "dockerfile"],
    placeholder: "Dockerfile",
  },
  { label: "Build context", path: ["build", "context"], placeholder: "." },
] as const;

export function KubernetesProjectFields({
  source,
  settings,
  onChange,
  disabled,
}: {
  source: string;
  settings: ReturnType<typeof readKubernetesProjectSettings>;
  onChange: (source: string) => void;
  disabled: boolean;
}) {
  const [mode, setMode] = useState("form");
  const id = useId();
  const label = t("Kubernetes settings");
  return (
    <>
      <FormField label={label} className={styles.settingsMode}>
        <SegmentedControl
          label={label}
          value={mode}
          onChange={setMode}
          options={[
            { value: "form", label: t("Form") },
            { value: "yaml", label: t("Edit") },
          ]}
          disabled={disabled}
        />
      </FormField>
      {mode === "yaml" ? (
        <div className={styles.yaml} data-kubernetes-project-editor>
          <Suspense fallback={<small role="status">{t("Loading…")}</small>}>
            <YamlEditor
              value={source}
              onChange={onChange}
              readOnly={disabled}
            />
          </Suspense>
        </div>
      ) : settings.error === undefined ? (
        <Form as="fieldset" columns={2} disabled={disabled}>
          {fields.map((field, index) => {
            const { path } = field;
            const [key, child] = path;
            const input = `${id}-${index}`;
            const { document, value: values } = settings;
            const section = values[key];
            const value = child
              ? (section as Record<string, unknown> | undefined)?.[child]
              : section;
            return (
              <FormField
                key={input}
                label={t(field.label)}
                htmlFor={input}
                fullWidth={"fullWidth" in field}
              >
                <input
                  id={input}
                  autoComplete="off"
                  value={typeof value === "string" ? value : ""}
                  placeholder={
                    "placeholder" in field ? field.placeholder : undefined
                  }
                  onChange={(event) => {
                    const value = event.target.value;
                    if (child && isAlias(document.get(key, true))) {
                      document.set(key, document.createNode(section));
                    }
                    if (value) document.setIn(path, value);
                    else document.deleteIn(path);
                    onChange(document.toString());
                  }}
                />
              </FormField>
            );
          })}
        </Form>
      ) : null}
      {settings.error !== undefined && (
        <ErrorNotice
          error={`${t("Invalid Kubernetes settings. Edit the YAML to continue.")} ${settings.error}`}
          className={styles.error}
          local
        />
      )}
    </>
  );
}
