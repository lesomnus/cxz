import { useRef, type ComponentProps } from "react";
import { useHiddenItems } from "./use-hidden-items";
import styles from "./panel-scroll.module.css";
import { ScrollArea } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";

// App labels and layout hooks only; behavior, styles and defaults live in Nox.
export function PanelScroll({
  className = "",
  classNames,
  scrollbarLabel,
  viewportRef: suppliedViewport,
  unreadSelector,
  ...props
}: ComponentProps<typeof ScrollArea> & { unreadSelector?: string }) {
  useLocale();
  const localViewport = useRef<HTMLDivElement>(null);
  const viewport = suppliedViewport ?? localViewport;
  const hidden = useHiddenItems(viewport, unreadSelector);
  return (
    <ScrollArea
      {...props}
      viewportRef={viewport}
      data-unread-above={hidden.above || undefined}
      data-unread-below={hidden.below || undefined}
      scrollbarLabel={scrollbarLabel ?? t("Panel scroll")}
      className={`panel-scroll-region ${styles.area} ${className}`}
      classNames={{
        viewport: "panel-scroll-content",
        content: "panel-scroll-inner",
        rail: "panel-scroll-rail",
        thumb: "panel-scroll-thumb",
        handle: "panel-scroll-handle",
        fade: "panel-scroll-fade",
        topFade: "panel-scroll-fade-top",
        bottomFade: "panel-scroll-fade-bottom",
        ...classNames,
      }}
    />
  );
}
