import { useId } from "react";
import { t } from "./i18n";

// Position zero unsets the key; its label follows the resolved default/global.
// Existing JSON values above eight remain visible until explicitly changed.
export function SettingSlider({
  label,
  value,
  inherited,
  disabled,
  onChange,
}: {
  label: string;
  value?: number;
  inherited: number;
  disabled?: boolean;
  onChange: (value: number | undefined) => void;
}) {
  const id = useId();
  const position = value === undefined ? 0 : Math.min(value, 8);
  return (
    <div className="setting-slider" data-muted={value === undefined}>
      <output htmlFor={id}>{value ?? inherited}</output>
      <input
        id={id}
        type="range"
        aria-label={label}
        min={0}
        max={8}
        step={1}
        value={position}
        disabled={disabled}
        aria-valuetext={
          value === undefined
            ? t("Inherited: {value}", { value: inherited })
            : String(value)
        }
        onChange={(event) => onChange(Number(event.target.value) || undefined)}
      />
      <div className="slider-ticks" aria-hidden="true">
        <span title={t("Reset to inherited value")}>↺</span>
        {Array.from({ length: 8 }, (_, i) => (
          <span key={i}>{i + 1}</span>
        ))}
      </div>
    </div>
  );
}
