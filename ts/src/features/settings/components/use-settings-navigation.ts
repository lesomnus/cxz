import { useCallback, useLayoutEffect, useRef, useState } from "react";
import type { SettingsTopic } from "#src/app/router.tsx";

export function useSettingsNavigation(
  topic: SettingsTopic,
  reveal: () => void,
) {
  const viewport = useRef<HTMLDivElement>(null);
  const layout = useRef<HTMLDivElement>(null);
  const revealRef = useRef(reveal);
  revealRef.current = reveal;
  const [active, setActive] = useState<SettingsTopic>(topic);
  const [compact, setCompact] = useState(true);
  const choose = useCallback(
    (value: string, behavior: ScrollBehavior = "smooth") => {
      revealRef.current();
      requestAnimationFrame(() => {
        const root = viewport.current;
        const section = layout.current?.querySelector<HTMLElement>(
          `[data-settings-topic="${value}"]`,
        );
        if (!root || !section) return;
        const inset = root.firstElementChild
          ? parseFloat(getComputedStyle(root.firstElementChild).paddingTop) || 0
          : 0;
        const top =
          root.scrollTop +
          section.getBoundingClientRect().top -
          root.getBoundingClientRect().top -
          inset;
        root.scrollTo({
          top: Math.max(0, top),
          behavior: matchMedia("(prefers-reduced-motion: reduce)").matches
            ? "auto"
            : behavior,
        });
      });
    },
    [],
  );
  useLayoutEffect(() => {
    const root = viewport.current;
    const content = layout.current;
    if (!root || !content) return;
    const sections = [
      ...content.querySelectorAll<HTMLElement>("[data-settings-topic]"),
    ];
    let frame = 0;
    const updateActive = () => {
      frame = 0;
      if (!root.clientHeight) return;
      const readingLine =
        root.getBoundingClientRect().top + root.clientHeight * 0.2;
      let current: HTMLElement | undefined = sections[0];
      for (const section of sections) {
        if (section.getBoundingClientRect().top <= readingLine)
          current = section;
      }
      const maximum = root.scrollHeight - root.clientHeight;
      if (maximum > 0 && maximum - root.scrollTop <= 1)
        current = sections.at(-1);
      if (current) setActive(current.dataset.settingsTopic as SettingsTopic);
    };
    const resize = () => {
      if (!root.clientWidth) {
        setCompact(true);
        return;
      }
      const menu = content.querySelector<HTMLElement>(
        "[data-settings-navigation]",
      );
      if (menu)
        setCompact(
          menu.getBoundingClientRect().left < root.getBoundingClientRect().left,
        );
      updateActive();
    };
    const scroll = () => {
      if (!frame) frame = requestAnimationFrame(updateActive);
    };
    const observer = new ResizeObserver(resize);
    observer.observe(root);
    observer.observe(content);
    for (const section of sections) observer.observe(section);
    root.addEventListener("scroll", scroll, { passive: true });
    resize();
    return () => {
      observer.disconnect();
      root.removeEventListener("scroll", scroll);
      cancelAnimationFrame(frame);
    };
  }, []);
  useLayoutEffect(() => {
    choose(topic, "auto");
  }, [topic, choose]);
  return { viewport, layout, active, compact, choose };
}

export type SettingsNavigation = ReturnType<typeof useSettingsNavigation>;
