import { useEffect, useLayoutEffect, useRef, type RefObject } from "react";

type Drift = {
  node: HTMLElement;
  point: () => Keyframe;
  frame: Keyframe;
  duration: number;
  animation?: Animation;
};
const between = (min: number, max: number) => min + Math.random() * (max - min);

// Layout stays in CSS. The browser interpolates random, slow paths; JavaScript
// chooses new waypoints only when a long animation finishes, never each frame.
export function ComposerAurora({
  active,
  anchor,
}: {
  active: boolean;
  anchor: RefObject<HTMLFormElement | null>;
}) {
  const viewport = useRef<HTMLDivElement>(null);
  const root = useRef<HTMLDivElement>(null);
  const latest = useRef(active);
  latest.current = active;
  const control = useRef<{ sync: () => void } | undefined>(undefined);
  useLayoutEffect(() => {
    const layer = viewport.current!;
    const composer = anchor.current!;
    const toolbar = composer.querySelector<HTMLElement>(".composer-toolbar")!;
    // Keep the field behind the conversation, anchored only on layout changes.
    // The bounded paint layer contains even the largest rotating orbs.
    const align = () => {
      const bounds = layer.getBoundingClientRect();
      const bar = toolbar.getBoundingClientRect();
      const node = root.current!;
      node.style.setProperty(
        "--aurora-anchor-left",
        `${bar.left - bounds.left}px`,
      );
      node.style.setProperty(
        "--aurora-anchor-top",
        `${bar.top - bounds.top}px`,
      );
      node.style.setProperty("--aurora-anchor-width", `${bar.width}px`);
    };
    const observer = new ResizeObserver(align);
    observer.observe(layer);
    observer.observe(composer);
    observer.observe(toolbar);
    align();
    return () => observer.disconnect();
  }, [anchor]);
  useEffect(() => {
    const node = root.current!;
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    const style = getComputedStyle(node);
    const timing = (token: string) => {
      const value = style.getPropertyValue(token).trim();
      return parseFloat(value) * (value.endsWith("ms") ? 1 : 1000);
    };
    const motions: Drift[] = [];
    let running = false,
      disposed = false,
      release = 0;
    function add(
      selector: string,
      token: string,
      frame: Keyframe,
      point: () => Keyframe,
    ) {
      for (const element of node.querySelectorAll<HTMLElement>(selector))
        motions.push({
          node: element,
          point,
          frame,
          duration: timing(token) * between(0.8, 1.2),
        });
    }
    add(
      ".aurora-cluster",
      "--aurora-drift-duration",
      { transform: "translate3d(0, 0, 0)" },
      () => ({
        transform: `translate3d(${between(-15, 15)}%, ${between(-16, 16)}%, 0)`,
      }),
    );
    add(
      ".aurora-orb",
      "--aurora-form-duration",
      { transform: "scale(1)", opacity: 0.7 },
      () => {
        const vanished = Math.random() < 0.2;
        return {
          transform: `rotate(${between(-32, 32)}deg) scale(${vanished ? 0.025 : between(0.45, 1.35)}, ${vanished ? 0.025 : between(0.4, 1.5)})`,
          opacity: vanished ? 0 : between(0.4, 0.8),
        };
      },
    );
    add(
      ".aurora-hotspot",
      "--aurora-focus-duration",
      { transform: "translate3d(0, 0, 0) scale(1)" },
      () => ({
        transform: `translate3d(${between(-24, 24)}%, ${between(-24, 24)}%, 0) scale(${between(0.45, 1)})`,
      }),
    );
    function wander(motion: Drift) {
      const frames = [motion.frame, ...Array.from({ length: 5 }, motion.point)];
      motion.animation?.cancel();
      motion.animation = motion.node.animate(
        frames.map((frame) => ({ ...frame, easing: "ease-in-out" })),
        { duration: motion.duration * between(0.85, 1.15), fill: "forwards" },
      );
      motion.frame = frames.at(-1)!;
      void motion.animation.finished.then(
        () => {
          if (!disposed && running) wander(motion);
        },
        () => {},
      );
    }
    function pause() {
      running = false;
      node.dataset.running = "false";
      for (const motion of motions) motion.animation?.pause();
    }
    function sync() {
      clearTimeout(release);
      // Resolve the initial tiny envelope before changing its target, including
      // mounting into a session that is already working.
      getComputedStyle(node.querySelector(".aurora-envelope")!).transform;
      node.dataset.active = String(latest.current);
      if (reduced.matches) {
        pause();
        for (const motion of motions) {
          motion.animation?.cancel();
          motion.animation = undefined;
        }
      } else if (latest.current) {
        running = true;
        node.dataset.running = "true";
        for (const motion of motions) {
          if (!motion.animation || motion.animation.playState === "finished")
            wander(motion);
          else motion.animation.play();
        }
      } else if (running) {
        // Keep orbit/drift alive through the visible contraction, then sleep.
        release = window.setTimeout(
          pause,
          timing("--aurora-envelope-duration"),
        );
      }
    }
    control.current = { sync };
    reduced.addEventListener("change", sync);
    sync();
    return () => {
      disposed = true;
      control.current = undefined;
      clearTimeout(release);
      reduced.removeEventListener("change", sync);
      for (const motion of motions) motion.animation?.cancel();
    };
  }, []);
  useEffect(() => control.current?.sync(), [active]);
  return (
    <div ref={viewport} className="composer-aurora-viewport" aria-hidden="true">
      <div
        ref={root}
        className="composer-aurora"
        data-active="false"
        data-running="false"
      >
        <div className="aurora-clusters">
          {[0, 1, 2].map((cluster) => (
            <div className="aurora-cluster" key={cluster}>
              <div className="aurora-envelope">
                {["green", "cyan", "violet"].map((color) => (
                  <div className="aurora-orbit" key={color}>
                    <div className={`aurora-orb aurora-${color}`}>
                      <span className="aurora-hotspot" />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
