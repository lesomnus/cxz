import { useMemo } from "react";
import { ElapsedTime } from "@lesomnus/nox";
import { highlightCodeLines } from "#src/shared/editor/code-syntax.ts";
import syntax from "#src/shared/content/markdown-code-block.module.css";
import {
  paletteVariables,
  defaultEditorSettings,
  type Palette,
} from "#src/shared/settings/editor-settings.ts";
import { useSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { backgroundCommand } from "./background-command";
import type { BackgroundTask } from "./background-tasks";
import styles from "./background-report.module.css";

export function BackgroundJobRow({ task }: { task: BackgroundTask }) {
  const { snapshot } = useSettings();
  const theme = useTheme();
  const palette =
    (snapshot.document["editor.colorPalette"] as Palette | undefined) ??
    defaultEditorSettings.colorPalette;
  const colors = useMemo(
    () => paletteVariables(palette, theme),
    [palette, theme],
  );
  const original = task.description || task.task_id;
  const tokens = useMemo(
    () =>
      highlightCodeLines(backgroundCommand(original), "bash").flatMap(
        (line, index) =>
          index ? [{ text: " ", className: "" }, ...line] : line,
      ),
    [original],
  );
  const start =
    task.started_at_ms && task.started_at_ms > 0
      ? task.started_at_ms
      : undefined;
  const duration =
    start !== undefined && !task.active && task.finished_at_ms
      ? Math.max(0, task.finished_at_ms - start)
      : undefined;
  const known = start !== undefined && (task.active || duration !== undefined);
  return (
    <div
      role="listitem"
      className={styles.job}
      data-finished={!task.active || undefined}
    >
      <code
        className={`${styles.command} ${syntax.syntax}`}
        title={original}
        style={colors}
      >
        {tokens.map((token, index) => (
          <span key={index} className={token.className || undefined}>
            {token.text}
          </span>
        ))}
      </code>
      <span className={styles.duration}>
        {known ? (
          <ElapsedTime
            startedAt={task.active ? start : undefined}
            durationMs={duration}
            dimLeadingZeros={false}
            label={t("Background task elapsed time")}
          />
        ) : (
          <span
            className={styles.unknown}
            role="img"
            aria-label={t("Background task duration unavailable")}
          >
            --:--:--
          </span>
        )}
      </span>
    </div>
  );
}
