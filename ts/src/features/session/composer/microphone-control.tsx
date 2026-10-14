import { useId } from "react";
import { Button, BadgedIcon } from "@lesomnus/cxz-ui";
import { t } from "#src/shared/i18n/i18n.ts";
import type { SpeechInput } from "#src/shared/browser/use-speech-input.ts";
import styles from "./microphone-control.module.css";

export function MicrophoneControl({
  speech,
  disabled,
  meterUnavailable,
}: {
  speech: SpeechInput;
  disabled: boolean;
  meterUnavailable: boolean;
}) {
  const hintId = useId();
  const label = speech.active ? t("Stop voice input") : t("Voice input");
  let hint = label;
  if (!speech.supported)
    hint = t(
      "Voice input is not supported in this browser. Use HTTPS or localhost in a supported browser.",
    );
  else if (speech.error === "local-unavailable")
    hint = t(
      "This browser cannot force local speech recognition. Turn off local recognition or use a supported browser.",
    );
  else if (speech.error === "local-language-unavailable")
    hint = t(
      "Local recognition is unavailable for this language. Install its browser language pack or turn off local recognition.",
    );
  else if (speech.error === "not-allowed")
    hint = t(
      "Microphone access was denied. Allow it in your browser settings.",
    );
  else if (speech.error === "audio-capture")
    hint = t("No microphone is available.");
  else if (speech.error === "no-speech")
    hint = t("No speech detected. Try again.");
  else if (speech.error === "network")
    hint = t(
      "The browser's speech recognition service is unavailable or unreachable. Microphone permission alone does not enable it.",
    );
  else if (speech.error === "service-not-allowed")
    hint = t(
      "This browser does not allow its speech recognition service. Check browser settings or use another supported browser.",
    );
  else if (speech.error === "language-not-supported")
    hint = t(
      "The browser's speech recognition service does not support the selected language.",
    );
  else if (speech.error === "aborted")
    hint = t("The browser interrupted voice input. Try again.");
  else if (speech.error)
    hint = t("Voice input could not start. See the browser error below.");
  else if (speech.stopping) hint = t("Finishing voice input…");
  else if (speech.starting) hint = t("Starting voice input…");
  else if (speech.active) hint = speech.interim;
  const showHint = Boolean(hint) || (meterUnavailable && speech.listening);
  return (
    <span
      className={styles.control}
      data-active={speech.active}
      data-notice={Boolean(speech.error)}
      data-error={speech.error || undefined}
      tabIndex={!speech.supported ? 0 : undefined}
      aria-describedby={!speech.supported ? hintId : undefined}
    >
      <Button
        className={`toolbar-button ${styles.microphone}`}
        type="button"
        aria-label={label}
        aria-pressed={speech.active}
        aria-describedby={showHint ? hintId : undefined}
        aria-keyshortcuts={speech.active ? "Escape" : undefined}
        disabled={disabled || !speech.supported || speech.stopping}
        onClick={speech.toggle}
      >
        <BadgedIcon
          className={styles.microphoneIcon}
          badgeClassName={styles.recordingPosition}
          badge={
            speech.listening ? (
              <span
                className={styles.recordingIndicator}
                data-voice-recording
              />
            ) : undefined
          }
        >
          <svg
            width="18"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            aria-hidden="true"
          >
            <rect x="9" y="2" width="6" height="12" rx="3" />
            <path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3m-4 0h8" />
          </svg>
        </BadgedIcon>
      </Button>
      {showHint && (
        <span id={hintId} role="tooltip" className={styles.hint}>
          <span role="status" aria-live="polite">
            {hint}
            {meterUnavailable && speech.listening && (
              <span className={styles.details}>
                {t(
                  "Audio visualization is unavailable. Voice input is still active.",
                )}
              </span>
            )}
            {speech.error && (
              <span className={styles.details}>
                {speech.error} · {speech.language}
                {speech.errorMessage && <span>{speech.errorMessage}</span>}
              </span>
            )}
          </span>
        </span>
      )}
    </span>
  );
}
