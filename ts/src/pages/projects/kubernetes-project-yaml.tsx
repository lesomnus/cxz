import { getSourceEditorLanguages } from "@lesomnus/nox/editor";
import * as monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { configureMonacoYaml, type MonacoYamlOptions } from "monaco-yaml";
import { useEffect, useState } from "react";
import { SourceEditor } from "#src/shared/editor/source-editor.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { kubernetesProjectSchema } from "./kubernetes-project-settings";
import YamlWorker from "./kubernetes-project-yaml.worker?worker";

let setup: Promise<void> | undefined;
function prepareYamlEditor() {
  return (setup ??= getSourceEditorLanguages()
    .then(() => {
      const yaml = configureMonacoYaml(
        {
          ...monaco,
          editor: {
            ...monaco.editor,
            // monaco-yaml uses the legacy worker factory. Monaco 0.55 takes
            // a Worker instance and reads its configuration from the first message.
            createWebWorker<T extends object>(
              options:
                | monaco.editor.IInternalWebWorkerOptions
                | { createData: MonacoYamlOptions },
            ) {
              if ("worker" in options)
                return monaco.editor.createWebWorker<T>(options);
              const worker = new YamlWorker();
              worker.postMessage(options.createData);
              return monaco.editor.createWebWorker<T>({ worker });
            },
          },
        },
        { enableSchemaRequest: false },
      );
      const paths = new Set<string>();
      const updateSchema = () =>
        yaml.update({
          schemas: paths.size
            ? [
                {
                  uri: "cxz://schemas/kubernetes-project",
                  fileMatch: [...paths],
                  schema: kubernetesProjectSchema,
                },
              ]
            : [],
        });

      // SourceEditor allocates its own model URI. Associate the schema with
      // the model inside the project editor before enabling its YAML service.
      monaco.editor.onDidCreateEditor((editor) => {
        if (
          !editor
            .getContainerDomNode()
            .closest("[data-kubernetes-project-editor]")
        )
          return;
        queueMicrotask(() => {
          const model = editor.getModel();
          if (!model || model.isDisposed()) return;
          const path = model.uri.path;
          paths.add(path);
          void updateSchema();
          monaco.editor.setModelLanguage(model, "yaml");
          model.onWillDispose(() => paths.delete(path));
        });
      });
    })
    .catch((error: unknown) => {
      setup = undefined;
      throw error;
    }));
}

export default function KubernetesProjectYaml({
  value,
  onChange,
  readOnly,
}: {
  value: string;
  onChange: (value: string) => void;
  readOnly: boolean;
}) {
  const [state, setState] = useState<"loading" | "ready" | "fallback">(
    "loading",
  );
  useEffect(() => {
    let disposed = false;
    prepareYamlEditor().then(
      () => {
        if (!disposed) setState("ready");
      },
      () => {
        if (!disposed) setState("fallback");
      },
    );
    return () => {
      disposed = true;
    };
  }, []);

  const label = t("Kubernetes project YAML");
  if (state === "loading") return <small role="status">{t("Loading…")}</small>;
  return (
    <SourceEditor
      path="kubernetes-project.yaml"
      languageId={state === "ready" ? "plaintext" : "yaml"}
      ariaLabel={label}
      value={value}
      readOnly={readOnly}
      onChange={onChange}
    />
  );
}
