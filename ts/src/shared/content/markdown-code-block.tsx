import { Fragment, useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { ValueMenu } from "@lesomnus/nox";
import { CopyButton } from "#src/shared/components/copy-button.tsx";
import {
  codeSyntax,
  codeSyntaxes,
  detectCodeSyntax,
  highlightCodeLines,
} from "#src/shared/editor/code-syntax.ts";
import {
  paletteVariables,
  resolveEditorSettings,
} from "#src/shared/settings/editor-settings.ts";
import { useSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./markdown-code-block.module.css";

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
  useEffect(() => setSelected(codeSyntax(language)), [language]);
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
  const palette = resolveEditorSettings(snapshot.document).colorPalette;
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
  const choices = codeSyntaxes.includes(selected)
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
              {line.map((token, tokenIndex) => (
                <span key={tokenIndex} className={token.className || undefined}>
                  {token.text}
                </span>
              ))}
            </Fragment>
          ))}
        </span>,
        code,
      )}
    </>
  );
}
