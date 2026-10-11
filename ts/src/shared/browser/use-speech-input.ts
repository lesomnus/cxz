import { useLayoutEffect, useRef, useState } from "react";
import { speechLanguage } from "./speech-language";

// Web Speech is not yet included in TypeScript's DOM declarations.
type RecognitionResult = {
  isFinal: boolean;
  0: { transcript: string };
};
type Recognition = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  onstart: (() => void) | null;
  onend: (() => void) | null;
  onerror: ((event: { error: string; message?: string }) => void) | null;
  onresult:
    | ((event: {
        resultIndex: number;
        results: ArrayLike<RecognitionResult>;
      }) => void)
    | null;
  start(): void;
  stop(): void;
  abort(): void;
};
type RecognitionConstructor = new () => Recognition;

function recognitionConstructor(): RecognitionConstructor | undefined {
  if (typeof window === "undefined" || !window.isSecureContext) return;
  const browser = window as Window & {
    SpeechRecognition?: RecognitionConstructor;
    webkitSpeechRecognition?: RecognitionConstructor;
  };
  return browser.SpeechRecognition ?? browser.webkitSpeechRecognition;
}

/** Native browser recognition only: no bundled model or app-server audio upload. */
export function useSpeechInput({
  disabled,
  onTranscript,
}: {
  disabled: boolean;
  onTranscript: (text: string) => void;
}) {
  const [Constructor] = useState(() => recognitionConstructor());
  const recognition = useRef<Recognition | null>(null);
  const callback = useRef(onTranscript);
  callback.current = onTranscript;
  const [phase, setPhase] = useState<
    "idle" | "starting" | "listening" | "stopping"
  >("idle");
  const [interim, setInterim] = useState("");
  const [error, setError] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const language =
    typeof navigator === "undefined"
      ? "en-US"
      : speechLanguage(navigator.language, navigator.languages);

  function reportError(code: string, message: string) {
    setError(code);
    setErrorMessage(message);
    // Diagnostic metadata only; never log audio or recognized text.
    console.warn("Voice input failed", { code, message, language });
    cancel();
  }

  function detach() {
    const current = recognition.current;
    recognition.current = null;
    if (!current) return;
    current.onstart = current.onend = current.onerror = current.onresult = null;
    current.abort();
  }
  function cancel() {
    detach();
    setPhase("idle");
    setInterim("");
  }
  useLayoutEffect(() => {
    if (disabled) cancel();
  }, [disabled]);
  useLayoutEffect(() => () => detach(), []);

  function toggle() {
    if (disabled || !Constructor) return;
    const current = recognition.current;
    if (current) {
      if (phase === "stopping") return;
      setPhase("stopping");
      // stop() requests a final result; abort() deliberately discards it.
      try {
        current.stop();
      } catch {
        cancel();
      }
      return;
    }
    setError("");
    setErrorMessage("");
    try {
      const next = new Constructor();
      recognition.current = next;
      next.lang = language;
      next.continuous = true;
      next.interimResults = true;
      const committed = new Set<number>();
      next.onstart = () => {
        if (recognition.current !== next) return;
        setPhase((phase) => (phase === "starting" ? "listening" : phase));
      };
      next.onresult = (event) => {
        if (recognition.current !== next) return;
        const final: string[] = [];
        const pending: string[] = [];
        for (let i = 0; i < event.results.length; i++) {
          const result = event.results[i];
          if (result.isFinal) {
            if (!committed.has(i)) {
              committed.add(i);
              final.push(result[0].transcript);
            }
          } else pending.push(result[0].transcript);
        }
        setInterim(pending.join(" "));
        const text = final.join(" ").trim();
        if (text) callback.current(text);
      };
      next.onerror = (event) => {
        if (recognition.current !== next) return;
        reportError(event.error, event.message ?? "");
      };
      next.onend = () => {
        if (recognition.current !== next) return;
        recognition.current = null;
        next.onstart = next.onend = next.onerror = next.onresult = null;
        setPhase("idle");
        setInterim("");
      };
      // Mark active while permission/start is pending, so another click stops it.
      setPhase("starting");
      next.start();
    } catch (error) {
      const name = error instanceof Error ? error.name : "";
      const code =
        name === "NotAllowedError"
          ? "not-allowed"
          : name === "NotSupportedError"
            ? "service-not-allowed"
            : "start-failed";
      reportError(code, error instanceof Error ? error.message : String(error));
    }
  }
  return {
    supported: Boolean(Constructor),
    active: phase !== "idle",
    stopping: phase === "stopping",
    starting: phase === "starting",
    interim,
    error,
    errorMessage,
    language,
    toggle,
    cancel,
  };
}

export type SpeechInput = ReturnType<typeof useSpeechInput>;
