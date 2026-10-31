import { CompactNumber } from "@lesomnus/nox";
import type { ComponentProps } from "react";
import { TabList } from "#src/shared/components/tab-list.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./question-tabs.module.css";

type QuestionTabsProps = Pick<
  ComponentProps<typeof TabList>,
  "prefix" | "panelId" | "value" | "choose"
> & {
  items: { id: string; answered: boolean }[];
  variant?: "filled" | "underline";
};

export function QuestionTabs({
  items,
  variant = "filled",
  ...props
}: QuestionTabsProps) {
  return (
    <TabList
      {...props}
      effect={variant === "underline" ? "none" : undefined}
      className={`question-tabs ${styles.tabs} ${variant === "underline" ? styles.underline : ""}`}
      label={t("Question steps")}
      items={items.map((item, index) => ({
        ...item,
        label: <CompactNumber value={index + 1} mode="integer" digits={2} />,
        ariaLabel: `Q${index + 1}`,
      }))}
    />
  );
}
