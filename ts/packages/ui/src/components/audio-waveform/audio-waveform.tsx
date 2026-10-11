import { useEffect, useRef } from "react";
import styles from "./audio-waveform.module.css";

/** A decorative, smoothed meter. The host owns audio capture and level sampling. */
export function AudioWaveform({
  active,
  readLevel,
  className = "",
  variant = "centered",
}: {
  active: boolean;
  readLevel: () => number;
  className?: string;
  /** Upward smoothly folds both polarities above the bottom baseline, using separate colors. */
  variant?: "centered" | "upward";
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const el = canvas.current;
    const ctx = el?.getContext("2d");
    if (!el || !ctx || !active) return;
    let frame = 0;
    let level = 0;
    let previous = performance.now();
    let phase = 0;
    let width = 0;
    let height = 0;
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    const size = () => {
      const rect = el.getBoundingClientRect();
      width = rect.width;
      height = rect.height;
      const ratio = Math.min(2, window.devicePixelRatio || 1);
      el.width = Math.ceil(width * ratio);
      el.height = Math.ceil(height * ratio);
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    };
    const resize = new ResizeObserver(size);
    resize.observe(el);
    size();
    function draw(now: number) {
      frame = 0;
      if (document.hidden) return;
      const elapsed = Math.min(64, now - previous);
      previous = now;
      const next = Math.min(1, Math.max(0, readLevel() || 0));
      level +=
        (next - level) * (1 - Math.exp(-elapsed / (next > level ? 80 : 220)));
      if (!reduced.matches) phase += elapsed * 0.003;
      ctx!.clearRect(0, 0, width, height);
      const style = getComputedStyle(el!);
      const polarities = variant === "upward" ? [1, -1] : [0];
      for (let layer = 0; layer < 3; layer++) {
        ctx!.globalAlpha = layer === 0 ? 0.8 : 0.24;
        ctx!.lineWidth = layer === 0 ? 1.5 : 1;
        for (const polarity of polarities) {
          ctx!.strokeStyle =
            polarity === 0
              ? style.color
              : style
                  .getPropertyValue(
                    polarity > 0
                      ? "--waveform-positive-color"
                      : "--waveform-negative-color",
                  )
                  .trim();
          ctx!.beginPath();
          for (let x = 0; x <= width; x += 2) {
            const t = x / Math.max(1, width);
            const envelope = Math.sin(Math.PI * t) ** 2;
            const wave =
              Math.sin(t * Math.PI * 5 - phase + layer * 0.7) +
              0.24 * Math.sin(t * Math.PI * 9 + phase * 0.6);
            const amplitude = envelope * level * height * (0.3 - layer * 0.07);
            // Squaring each half-wave makes its derivative zero at the baseline.
            // Normalize by the combined sine peaks to retain the original height.
            const folded = Math.max(0, polarity * wave) ** 2 / 1.24;
            const y =
              variant === "upward"
                ? height - folded * amplitude
                : height / 2 + wave * amplitude;
            if (x === 0) ctx!.moveTo(x, y);
            else ctx!.lineTo(x, y);
          }
          ctx!.stroke();
        }
      }
      frame = requestAnimationFrame(draw);
    }
    const visibility = () => {
      cancelAnimationFrame(frame);
      frame = 0;
      if (!document.hidden) {
        previous = performance.now();
        frame = requestAnimationFrame(draw);
      }
    };
    document.addEventListener("visibilitychange", visibility);
    visibility();
    return () => {
      cancelAnimationFrame(frame);
      resize.disconnect();
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [active, readLevel, variant]);
  return (
    <canvas
      ref={canvas}
      className={`${styles.waveform} ${className}`}
      data-active={active}
      data-variant={variant}
      aria-hidden="true"
    />
  );
}
