import { Icon } from "#src/shared/icons/icon.tsx";
import { useId } from "react";
import { Button } from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";

export function CopyButton({
  value,
  className = "",
}: {
  value: string;
  className?: string;
}) {
  useLocale();
  const tooltip = useId();
  return (
    <span className={`meta-popover ${className}`}>
      <Button
        type="button"
        variant="toolbar"
        className="copy"
        aria-label={t("Copy")}
        aria-describedby={tooltip}
        onClick={() => navigator.clipboard.writeText(value).catch(() => {})}
      >
        <Icon name="copy" width="16" height="16" aria-hidden="true" />
      </Button>
      <span id={tooltip} className="meta-tooltip" role="tooltip">
        {t("Copy")}
      </span>
    </span>
  );
}
