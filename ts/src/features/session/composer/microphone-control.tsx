import { useId } from "react";
import { Button, ValueMenu, type ValueOption } from "@lesomnus/cxz-ui";
import { t } from "#src/shared/i18n/i18n.ts";
import type { SpeechInput } from "#src/shared/browser/use-speech-input.ts";
import styles from "./microphone-control.module.css";
import { voiceLanguages } from "./voice-language";

export function MicrophoneControl({
  speech,
  disabled,
  language,
  onLanguage,
  languageError,
  meterUnavailable,
}: {
  speech: SpeechInput;
  disabled: boolean;
  language?: string;
  onLanguage: (language: string | undefined) => void;
  languageError: string;
  meterUnavailable: boolean;
}) {
  const hintId = useId();
  const label = speech.active ? t("Stop voice input") : t("Voice input");
  let hint = label;
  if (!speech.supported)
    hint = t(
      "Voice input is not supported in this browser. Use HTTPS or localhost in a supported browser.",
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
  else if (speech.active) hint = speech.interim || t("Listening…");
  if (languageError) hint = languageError;
  const options: ValueOption[] = [
    { value: "browser", label: t("Browser language"), muted: true },
    ...voiceLanguages.map(([value, label]) => ({ value, label })),
  ];
  if (language && !options.some((option) => option.value === language))
    options.push({ value: language, label: language });

  return (
    <span className={styles.controls}>
      <span
        className={styles.control}
        data-active={speech.active}
        data-notice={Boolean(speech.error || languageError)}
        data-error={speech.error || undefined}
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
      </span>
      <span className={styles.language}>
        <ValueMenu
          label={t("Voice language")}
          variant="compact"
          value={language || "browser"}
          display={speech.language.toUpperCase()}
          options={options}
          muted={!language}
          minMenuWidth={180}
          disabled={disabled || !speech.supported || speech.active}
          disabledReason={
            speech.active
              ? t("Stop voice input before changing the language.")
              : undefined
          }
          choose={(value) =>
            onLanguage(value === "browser" ? undefined : value)
          }
        />
      </span>
    </span>
  );
}
