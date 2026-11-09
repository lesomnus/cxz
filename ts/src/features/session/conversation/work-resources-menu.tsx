import type { ComponentProps } from "react";
import { AnchoredMenu } from "#src/shared/components/anchored-menu.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "#src/shared/components/anchored-menu.module.css";

export function WorkResourcesMenu({
  partial,
  ...props
}: Omit<ComponentProps<typeof AnchoredMenu>, "footer"> & { partial: boolean }) {
  useLocale();
  return (
    <AnchoredMenu
      {...props}
      footer={
        partial && (
          <p className={styles.note}>
            {t("Only the loaded conversation range is shown.")}
          </p>
        )
      }
    />
  );
}
