import { Icon } from "#src/shared/icons/icon.tsx";
import { Popover } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./field-label.module.css";

export function FieldLabel({
  label,
  description,
}: {
  label: string;
  description?: string;
}) {
  useLocale();
  return (
    <span className={styles.label}>
      <span>{label}</span>
      {description && (
        <span className={styles.help}>
          <Popover content={description}>
            <Icon
              name="info"
              role="img"
              aria-label={t("About {label}", { label })}
            />
          </Popover>
        </span>
      )}
    </span>
  );
}
