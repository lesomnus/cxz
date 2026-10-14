import type { ComponentProps } from "react";
import { ScrollArea } from "@lesomnus/cxz-ui";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";

// App labels and layout hooks only; behavior, styles and defaults live in cxz-ui.
export function PanelScroll({
  className = "",
  classNames,
  scrollbarLabel,
  ...props
}: ComponentProps<typeof ScrollArea>) {
  useLocale();
  return (
    <ScrollArea
      {...props}
      scrollbarLabel={scrollbarLabel ?? t("Panel scroll")}
      className={`panel-scroll-region ${className}`}
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
