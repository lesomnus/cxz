import { useState } from "react";
import { ToggleButton } from "@lesomnus/nox";
import { useSettings } from "#src/shared/settings/settings.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./composer-options.module.css";

export function ComposerOptions() {
  useLocale();
  const { store, snapshot } = useSettings();
  const [error, setError] = useState("");
  const [focused, setFocused] = useState(0);
  const controls = snapshot.document["composer.showControlCharacters"] === true;
  const wrap = snapshot.document["composer.wordWrap"] !== false;
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
          onPressedChange={(pressed) =>
            toggle("composer.showControlCharacters", pressed)
          }
        >
          {({ pressed }) => (
            <EditorOptionIcon option="controls" pressed={pressed} />
          )}
        </ToggleButton>
        <ToggleButton
          type="button"
          variant="toolbar"
          aria-label={t("Word wrap")}
          title={t("Word wrap")}
          pressed={wrap}
          tabIndex={focused === 1 ? 0 : -1}
          onFocus={() => setFocused(1)}
          onPressedChange={(pressed) => toggle("composer.wordWrap", pressed)}
        >
          {({ pressed }) => (
            <EditorOptionIcon option="wrap" pressed={pressed} />
          )}
        </ToggleButton>
      </div>
      {error && (
        <small className={styles.error} role="alert">
          {error}
        </small>
      )}
    </>
  );
}

function EditorOptionIcon({
  option,
  pressed,
}: {
  option: "controls" | "wrap";
  pressed: boolean;
}) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.4"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {option === "controls" ? (
        <>
          <path d="M11 16V4H8a3 3 0 0 0 0 6h3m3-6v12M8 4h8" />
          {!pressed && <path d="m3 3 14 14" />}
        </>
      ) : pressed ? (
        <path d="M3 4h14M3 9h11a3 3 0 0 1 0 6h-4m3-3-3 3 3 3M3 14h3" />
      ) : (
        <path d="M3 4h14M3 9h14M3 14h9m2-2 3 2-3 2" />
      )}
    </svg>
  );
}
