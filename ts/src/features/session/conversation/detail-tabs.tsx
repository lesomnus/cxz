import { useId, useRef, useState } from "react";
import type * as Monaco from "monaco-editor/esm/vs/editor/editor.api.js";
import { Button } from "@lesomnus/cxz-ui";
import { CopyButton } from "#src/shared/components/copy-button.tsx";
import { SourceEditor } from "#src/shared/editor/source-editor.tsx";
import { detectCodeSyntax } from "#src/features/session/composer/composer-code.ts";

export type DetailSection = {
  id: string;
  label: string;
  value: string;
  language?: string;
};

// Detection is shared with the composer; map its grammar names to Monaco IDs.
function detectedLanguage(value: string) {
  const syntax = detectCodeSyntax(value);
  return syntax === "bash" ? "shell" : syntax;
}

export function DetailTabs({ sections }: { sections: DetailSection[] }) {
  const prefix = useId();
  const [selected, setSelected] = useState(sections[0]?.id);
  const views = useRef(
    new Map<string, Monaco.editor.ICodeEditorViewState | null>(),
  );
  const active =
    sections.find((section) => section.id === selected) ?? sections[0];
  if (!active) return null;
  const panel = `${prefix}-panel`;
  const language = active.language ?? detectedLanguage(active.value);
  return (
    <div className="detail-tabs">
      <div className="detail-tab-bar">
        <div className="detail-tab-list" role="tablist">
          {sections.map((section, index) => (
            <Button
              key={section.id}
              id={`${prefix}-${section.id}`}
              type="button"
              role="tab"
              aria-selected={section.id === active.id}
              aria-controls={panel}
              tabIndex={section.id === active.id ? 0 : -1}
              onClick={() => setSelected(section.id)}
              onKeyDown={(event) => {
                let next: number;
                if (event.key === "ArrowRight")
                  next = (index + 1) % sections.length;
                else if (event.key === "ArrowLeft")
                  next = (index - 1 + sections.length) % sections.length;
                else if (event.key === "Home") next = 0;
                else if (event.key === "End") next = sections.length - 1;
                else return;
                event.preventDefault();
                setSelected(sections[next].id);
                document
                  .getElementById(`${prefix}-${sections[next].id}`)
                  ?.focus();
              }}
            >
              {section.label}
            </Button>
          ))}
        </div>
        <CopyButton value={active.value} className="detail-copy" />
      </div>
      <div
        className="detail-editor"
        data-language={language}
        id={panel}
        role="tabpanel"
        aria-labelledby={`${prefix}-${active.id}`}
      >
        <SourceEditor
          key={active.id}
          value={active.value}
          path={active.id}
          languageId={language}
          ariaLabel={active.label}
          readOnly
          view={views.current.get(active.id)}
          onDispose={(view) => views.current.set(active.id, view)}
        />
      </div>
    </div>
  );
}
