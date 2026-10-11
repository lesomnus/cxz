import { useId } from "react";
import { Button } from "@lesomnus/cxz-ui";
import { t } from "#src/shared/i18n/i18n.ts";
import type { SpeechInput } from "#src/shared/browser/use-speech-input.ts";
import styles from "./microphone-control.module.css";

export function MicrophoneControl({
  speech,
  disabled,
}: {
  speech: SpeechInput;
  disabled: boolean;
}) {
  const hintId = useId();
  const label = speech.active ? t("Stop voice input") : t("Voice input");
  let hint = label;
  if (!speech.supported)
    hint = t(
      "Voice input is not supported in this browser. Use HTTPS or localhost in a supported browser.",
    );
  else if (
    speech.error === "not-allowed" ||
    speech.error === "service-not-allowed"
  )
    hint = t(
      "Microphone access was denied. Allow it in your browser settings.",
    );
  else if (speech.error === "audio-capture")
    hint = t("No microphone is available.");
  else if (speech.error === "no-speech")
    hint = t("No speech detected. Try again.");
  else if (speech.error)
    hint = t("Voice input failed. Check your connection and try again.");
  else if (speech.stopping) hint = t("Finishing voice input…");
  else if (speech.active) hint = speech.interim || t("Listening…");

  return (
    <span
      className={styles.control}
      data-active={speech.active}
      data-notice={Boolean(speech.error)}
      tabIndex={!speech.supported ? 0 : undefined}
      aria-describedby={!speech.supported ? hintId : undefined}
    >
      <Button
        className={`toolbar-button ${styles.microphone}`}
        type="button"
        aria-label={label}
        aria-pressed={speech.active}
        aria-describedby={hintId}
        disabled={disabled || !speech.supported || speech.stopping}
        onClick={speech.toggle}
      >
        <svg
          width="18"
          height="18"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          aria-hidden="true"
        >
          <rect x="9" y="2" width="6" height="12" rx="3" />
          <path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3m-4 0h8" />
        </svg>
      </Button>
      <span id={hintId} role="tooltip" className={styles.hint}>
        <span role="status" aria-live="polite">
          {hint}
        </span>
      </span>
    </span>
  );
}
