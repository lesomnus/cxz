import { useCallback, useEffect, useRef, useState } from "react";

/** Meter-only capture: never connect to speakers, record, store or upload audio. */
export function useMicrophoneLevel(active: boolean) {
  const meter = useRef<{
    analyser: AnalyserNode;
    samples: Float32Array<ArrayBuffer>;
  } | null>(null);
  const [status, setStatus] = useState<
    "idle" | "loading" | "ready" | "unavailable"
  >("idle");
  const readLevel = useCallback(() => {
    const current = meter.current;
    if (!current) return 0;
    current.analyser.getFloatTimeDomainData(current.samples);
    let sum = 0;
    for (const sample of current.samples) sum += sample * sample;
    // Map speech RMS into a useful visual range; silence remains zero.
    return Math.min(1, Math.sqrt(sum / current.samples.length) * 8);
  }, []);

  useEffect(() => {
    if (!active) {
      setStatus("idle");
      return;
    }
    let disposed = false;
    let stream: MediaStream | undefined;
    let context: AudioContext | undefined;
    let source: MediaStreamAudioSourceNode | undefined;
    let analyser: AnalyserNode | undefined;
    function release() {
      disposed = true;
      meter.current = null;
      source?.disconnect();
      analyser?.disconnect();
      stream?.getTracks().forEach((track) => track.stop());
      if (context && context.state !== "closed")
        void context.close().catch(() => {});
    }
    setStatus("loading");
    void (async () => {
      try {
        if (
          !navigator.mediaDevices?.getUserMedia ||
          typeof AudioContext === "undefined"
        )
          throw new Error("Audio metering unavailable");
        context = new AudioContext();
        stream = await navigator.mediaDevices.getUserMedia({ audio: true });
        // A permission request may resolve after stop or a session switch.
        if (disposed) {
          stream.getTracks().forEach((track) => track.stop());
          return;
        }
        source = context.createMediaStreamSource(stream);
        analyser = context.createAnalyser();
        analyser.fftSize = 512;
        source.connect(analyser);
        await context.resume();
        if (disposed) return;
        meter.current = {
          analyser,
          samples: new Float32Array(analyser.fftSize),
        };
        stream.getAudioTracks().forEach((track) =>
          track.addEventListener(
            "ended",
            () => {
              if (!disposed) {
                setStatus("unavailable");
                release();
              }
            },
            { once: true },
          ),
        );
        setStatus("ready");
      } catch {
        if (!disposed) {
          setStatus("unavailable");
          release();
        }
      }
    })();
    return release;
  }, [active]);
  return { readLevel, status };
}
