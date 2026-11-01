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
  variant?: "filled" | "history";
};

export function QuestionTabs({
  items,
  variant = "filled",
  ...props
}: QuestionTabsProps) {
  return (
    <TabList
      {...props}
      className={`question-tabs ${styles.tabs} ${variant === "history" ? styles.history : ""}`}
      label={t("Question steps")}
      items={items.map((item, index) => ({
        ...item,
        answered: variant === "filled" ? item.answered : undefined,
        label: <CompactNumber value={index + 1} mode="integer" digits={2} />,
        ariaLabel: `Q${index + 1}`,
      }))}
    />
  );
}
