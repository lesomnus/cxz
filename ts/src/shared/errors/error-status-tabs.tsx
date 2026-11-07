import { SegmentedControl, CompactNumber } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
export function ErrorStatusTabs({
  active,
  history,
  tab,
  onChange,
}: {
  active: number;
  history: number;
  tab: string;
  onChange: (tab: string) => void;
}) {
  useLocale();
  return (
    <SegmentedControl
      label={t("Error status")}
      size="compact"
      value={tab}
      onChange={onChange}
      options={[
        {
          value: "active",
          label: `${t("Active")} ${active}`,
          display: (
            <>
              {t("Active")}
              <CompactNumber value={active} mode="integer" digits={3} />
            </>
          ),
        },
        {
          value: "history",
          label: `${t("History")} ${history}`,
          display: (
            <>
              {t("History")}
              <CompactNumber value={history} mode="integer" digits={3} />
            </>
          ),
        },
      ]}
    />
  );
}
