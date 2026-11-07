import { Icon } from "#src/shared/icons/icon.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useState } from "react";
import { SegmentedControl, ToggleButton } from "@lesomnus/nox";
import { useSettings } from "#src/shared/settings/settings.ts";
import {
  resolveEditorSettings,
  type EditorScope,
} from "#src/shared/settings/editor-settings.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./editor-options.module.css";

export function EditorOptions({ scope = "global" }: { scope?: EditorScope }) {
  useLocale();
  const { store, snapshot } = useSettings();
  const [error, setError] = useState("");
  const [focused, setFocused] = useState(0);
  const prefix = scope === "session" ? "composer" : "editor";
  const controlsKey = `${prefix}.showControlCharacters`;
  const wrapKey = `${prefix}.wordWrap`;
  const controls = snapshot.document[controlsKey] === true;
  const wrap =
    scope === "session"
      ? snapshot.document[wrapKey] !== false
      : snapshot.document[wrapKey] === true;
  const editorSettings = resolveEditorSettings(snapshot.document, scope);
  function toggle(key: string, value: boolean) {
    try {
      store.set(key, value);
      setError("");
    } catch (error) {
      setError(String(error));
    }
  }
  return (
    <>
      <div className={styles.footer}>
        <SegmentedControl
          label={t("Tab input")}
          size="compact"
          iconOnly
          value={editorSettings.insertSpaces ? "spaces" : "tab"}
          options={[
            {
              value: "tab",
              label: t("Tab character"),
              icon: <EditorOptionIcon option="indent" pressed={false} />,
            },
            {
              value: "spaces",
              label: t("Spaces"),
              icon: <EditorOptionIcon option="indent" pressed />,
            },
          ]}
          onChange={(value) =>
            toggle(
              `${scope === "session" ? "session." : ""}editor.insertSpaces`,
              value === "spaces",
            )
          }
        />
        <div
          className={styles.toolbar}
          role="toolbar"
          aria-label={t("Editor options")}
          onKeyDown={(event) => {
            if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key))
              return;
            const buttons = [
              ...event.currentTarget.querySelectorAll<HTMLButtonElement>(
                "button:not(:disabled)",
              ),
            ];
            const index = buttons.indexOf(
              document.activeElement as HTMLButtonElement,
            );
            const next =
              event.key === "Home"
                ? 0
                : event.key === "End"
                  ? buttons.length - 1
                  : (index +
                      (event.key === "ArrowLeft" ? -1 : 1) +
                      buttons.length) %
                    buttons.length;
            event.preventDefault();
            buttons[next]?.focus();
          }}
        >
          <ToggleButton
            type="button"
            variant="toolbar"
            aria-label={t("Show control characters")}
            title={t("Show control characters")}
            pressed={controls}
            tabIndex={focused === 0 ? 0 : -1}
            onFocus={() => setFocused(0)}
            onPressedChange={(pressed) => toggle(controlsKey, pressed)}
          >
            {({ pressed }) => (
              <EditorOptionIcon option="controls" pressed={pressed} />
            )}
          </ToggleButton>
          <ToggleButton
            type="button"
            variant="toolbar"
            aria-label={t("Word wrap")}
            aria-keyshortcuts={scope === "session" ? "Alt+Z" : undefined}
            title={
              scope === "session" ? `${t("Word wrap")} · Alt+Z` : t("Word wrap")
            }
            pressed={wrap}
            tabIndex={focused === 1 ? 0 : -1}
            onFocus={() => setFocused(1)}
            onPressedChange={(pressed) => toggle(wrapKey, pressed)}
          >
            {({ pressed }) => (
              <EditorOptionIcon option="wrap" pressed={pressed} />
            )}
          </ToggleButton>
        </div>
      </div>
      {error && (
        <ErrorNotice as="small" className={styles.error} error={error} />
      )}
    </>
  );
}

function EditorOptionIcon({
  option,
  pressed,
}: {
  option: "controls" | "wrap" | "indent";
  pressed: boolean;
}) {
  return (
    <Icon
      name={
        option === "controls"
          ? pressed
            ? "show-controls"
            : "hide-controls"
          : option === "indent"
            ? pressed
              ? "indent-spaces"
              : "indent-tab"
            : pressed
              ? "word-wrap"
              : "no-word-wrap"
      }
      width="16"
      height="16"
      aria-hidden="true"
    />
  );
}
