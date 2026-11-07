import { CompactNumber, Tabs, type TabsProps } from "@lesomnus/nox";
import { Icon } from "#src/shared/icons/icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import styles from "./question-tabs.module.css";

type QuestionTabsProps = Pick<TabsProps, "prefix" | "panelId" | "value"> & {
  items: { id: string; answered: boolean }[];
  choose: (id: string) => void;
  variant?: "filled" | "history";
};

export function QuestionTabs({
  items,
  variant = "filled",
  choose,
  ...props
}: QuestionTabsProps) {
  return (
    <Tabs
      {...props}
      className={`question-tabs ${styles.tabs} ${variant === "history" ? styles.history : ""}`}
      aria-label={t("Question steps")}
      onValueChange={choose}
      items={items.map((item, index) => ({
        ...item,
        icon:
          variant === "filled" ? (
            <span className="tab-answered" data-answered={item.answered}>
              <Icon
                name={item.answered ? "question-answered" : "question-open"}
                width="16"
                height="16"
              />
            </span>
          ) : undefined,
        label: <CompactNumber value={index + 1} mode="integer" digits={2} />,
        ariaLabel: `Q${index + 1}`,
      }))}
    />
  );
}
