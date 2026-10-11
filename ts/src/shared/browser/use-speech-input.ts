import { useLayoutEffect, useRef, useState } from "react";

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
  onerror: ((event: { error: string }) => void) | null;
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
  const [phase, setPhase] = useState<"idle" | "listening" | "stopping">("idle");
  const [interim, setInterim] = useState("");
  const [error, setError] = useState("");

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
    try {
      const next = new Constructor();
      recognition.current = next;
      next.lang = navigator.language || "en-US";
      next.continuous = true;
      next.interimResults = true;
      const committed = new Set<number>();
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
        setError(event.error);
        cancel();
      };
      next.onend = () => {
        if (recognition.current !== next) return;
        recognition.current = null;
        next.onstart = next.onend = next.onerror = next.onresult = null;
        setPhase("idle");
        setInterim("");
      };
      // Mark active while permission/start is pending, so another click stops it.
      setPhase("listening");
      next.start();
    } catch {
      cancel();
      setError("start-failed");
    }
  }
  return {
    supported: Boolean(Constructor),
    active: phase !== "idle",
    stopping: phase === "stopping",
    interim,
    error,
    toggle,
    cancel,
  };
}

export type SpeechInput = ReturnType<typeof useSpeechInput>;
