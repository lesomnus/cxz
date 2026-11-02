import { Fragment, useEffect, useLayoutEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { Button, ScrollArea, ValueMenu } from "@lesomnus/nox";
import { CopyButton } from "#src/shared/components/copy-button.tsx";
import {
  codeSyntax,
  codeSyntaxes,
  detectCodeSyntax,
  highlightCodeLines,
} from "#src/shared/editor/code-syntax.ts";
import {
  paletteVariables,
  defaultEditorSettings,
  type Palette,
} from "#src/shared/settings/editor-settings.ts";
import { useSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./markdown-code-block.module.css";
import { useMermaid } from "./use-mermaid";

export function MarkdownCodeBlock({
  container,
  code,
  text,
  language,
  surface = "default",
}: {
  container: HTMLElement;
  code: HTMLElement;
  text: string;
  language: string;
  surface?: "default" | "input";
}) {
  useLocale();
  const [selected, setSelected] = useState(codeSyntax(language));
  const [showSource, setShowSource] = useState(false);
  useEffect(() => setSelected(codeSyntax(language)), [language]);
  const isMermaid = selected === "mermaid";
  const diagram = useMermaid(text, isMermaid);
  const showDiagram = isMermaid && !showSource;
  const diagramReady = showDiagram && diagram.status === "ready";
  useLayoutEffect(() => {
    const pre = code.parentElement!;
    pre.hidden = diagramReady;
    return () => {
      pre.hidden = false;
    };
  }, [code, diagramReady]);
  const syntax = useMemo(
    () => (selected === "auto" ? detectCodeSyntax(text) : selected),
    [selected, text],
  );
  const tokens = useMemo(
    () => highlightCodeLines(text, syntax),
    [text, syntax],
  );
  const { snapshot } = useSettings();
  const theme = useTheme();
  // Highlighting needs only the palette, not font resolution (a layout read).
  const palette =
    (snapshot.document["editor.colorPalette"] as Palette | undefined) ??
    defaultEditorSettings.colorPalette;
  const colors = paletteVariables(palette, theme);
  // Lift contrast on the user's raised gray surface without discarding the
  // chosen palette's hues. Theme foreground also gives darker ink in light mode.
  const syntaxColors =
    surface === "input"
      ? Object.fromEntries(
          Object.entries(colors).map(([key, value]) => [
            key,
            `color-mix(in srgb, ${value} 82%, var(--shade-237))`,
          ]),
        )
      : colors;
  const choices =
    codeSyntax(language) === "mermaid"
      ? ["mermaid", ...codeSyntaxes]
      : codeSyntaxes.includes(selected)
        ? codeSyntaxes
        : [selected, ...codeSyntaxes];
  return (
    <>
      {createPortal(
        <div
          className={styles.toolbar}
          onClick={(event) => event.stopPropagation()}
          onDoubleClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <ValueMenu
            label={t("Language")}
            variant="compact"
            value={selected}
            options={choices.map((value) => ({
              value,
              label:
                value === "auto" ? t("Auto · {syntax}", { syntax }) : value,
            }))}
            choose={setSelected}
          />
          {isMermaid && (
            <Button
              type="button"
              variant="toolbar"
              className={styles.viewToggle}
              aria-label={showSource ? t("View diagram") : t("View source")}
              onClick={() => setShowSource((value) => !value)}
            >
              {showSource ? t("Diagram") : t("Source")}
            </Button>
          )}
          <CopyButton value={text} />
        </div>,
        container,
      )}
      {createPortal(
        <span
          className={styles.syntax}
          style={syntaxColors}
          data-language={syntax}
        >
          {tokens.map((line, index) => (
            <Fragment key={index}>
              {index > 0 && "\n"}
              {line.map((token, tokenIndex) =>
                token.className ? (
                  <span key={tokenIndex} className={token.className}>
                    {token.text}
                  </span>
                ) : (
                  token.text
                ),
              )}
            </Fragment>
          ))}
        </span>,
        code,
      )}
      {showDiagram &&
        createPortal(
          diagram.status === "ready" ? (
            <ScrollArea
              className={styles.diagram}
              viewportClassName={styles.diagramViewport}
              contentClassName={styles.diagramContent}
              scrollbarLabel={t("Scroll {label}", {
                label: t("Mermaid diagram"),
              })}
            >
              <img
                className={styles.diagramImage}
                src={diagram.url}
                width={diagram.width}
                height={diagram.height}
                alt={
                  text.match(/^\s*accTitle:\s*(.+)$/m)?.[1] ??
                  t("Mermaid diagram")
                }
              />
            </ScrollArea>
          ) : (
            <span className={styles.diagramStatus} role="status">
              {diagram.status === "error"
                ? t("Unable to render this Mermaid diagram. Showing source.")
                : t("Rendering diagram…")}
            </span>
          ),
          container,
        )}
    </>
  );
}
