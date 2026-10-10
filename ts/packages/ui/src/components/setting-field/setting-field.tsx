import styles from "./setting-field.module.css";
import { useId, type ReactNode } from "react";

// Setting metadata stays above its control; supplementary guidance is optional.
export function SettingField({
  title,
  settingId,
  summary,
  children,
  details,
}: {
  title: string;
  settingId: string;
  summary: string;
  children: ReactNode;
  details?: ReactNode;
}) {
  const heading = useId();
  const identifier = `${heading}-setting-id`;
  return (
    <div
      className={`${styles.setting_row} setting-row`}
      role="group"
      aria-labelledby={`${heading} ${identifier}`}
    >
      <strong className="setting-title" id={heading}>
        {title}
      </strong>
      <code className="setting-id" id={identifier}>
        {settingId}
      </code>
      <p className="setting-summary">{summary}</p>
      <div className="setting-control">{children}</div>
      {details && <div className="setting-details">{details}</div>}
    </div>
  );
}
