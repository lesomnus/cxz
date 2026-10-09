import { useId } from "react";
import { Button } from "./button";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";

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
        className="copy"
        aria-label={t("Copy")}
        aria-describedby={tooltip}
        onClick={() => navigator.clipboard.writeText(value).catch(() => {})}
      >
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <rect x="8" y="8" width="12" height="12" rx="2" />
          <path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3" />
        </svg>
      </Button>
      <span id={tooltip} className="meta-tooltip" role="tooltip">
        {t("Copy")}
      </span>
    </span>
  );
}
