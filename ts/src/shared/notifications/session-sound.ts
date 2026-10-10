import completeURL from "./sounds/complete.wav?url";
import questionURL from "./sounds/attention.wav?url";
import type { SessionAlert } from "#src/features/session/model/session-attention.ts";

// Original local PCM cues shared with the TUI. Never request OS permissions or
// queue an alert that happened before the browser allowed audio.
export function sessionSound(enabled: () => boolean) {
  let context: AudioContext | undefined;
  let buffers: Partial<Record<SessionAlert, AudioBuffer>> = {};
  let disposed = false;
  let nextTime = 0;
  const abort = new AbortController();
  const unlock = () => {
    if (disposed || !enabled()) return;
    try {
      if (!context) {
        context = new AudioContext();
        for (const [kind, url] of [
          ["complete", completeURL],
          ["question", questionURL],
        ] as const) {
          const audio = context;
          void fetch(url, { signal: abort.signal })
            .then((r) => {
              if (!r.ok) throw new Error("Audio unavailable");
              return r.arrayBuffer();
            })
            .then((bytes) => audio.decodeAudioData(bytes))
            .then((buffer) => {
              if (!disposed) buffers[kind] = buffer;
            })
            .catch(() => {});
        }
      }
      if (context.state === "suspended") void context.resume().catch(() => {});
    } catch {
      /* Audio support/autoplay policy never blocks the workspace. */
    }
  };
  document.addEventListener("pointerdown", unlock, true);
  document.addEventListener("keydown", unlock, true);
  return {
    play(kind: SessionAlert) {
      const buffer = buffers[kind];
      if (!enabled() || disposed || context?.state !== "running" || !buffer)
        return;
      // Serialize a small burst, then drop excess alerts instead of building a
      // long sound queue when several sessions finish together.
      if (nextTime > context.currentTime + 1) return;
      const source = context.createBufferSource();
      const gain = context.createGain();
      source.buffer = buffer;
      gain.gain.value = 0.35;
      source.connect(gain).connect(context.destination);
      const start = Math.max(context.currentTime, nextTime);
      nextTime = start + buffer.duration;
      source.start(start);
      source.onended = () => {
        source.disconnect();
        gain.disconnect();
      };
    },
    dispose() {
      disposed = true;
      abort.abort();
      document.removeEventListener("pointerdown", unlock, true);
      document.removeEventListener("keydown", unlock, true);
      buffers = {};
      void context?.close().catch(() => {});
    },
  };
}
