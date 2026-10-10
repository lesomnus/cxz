import styles from "./segmented-control.module.css";
import { useId, type CSSProperties } from "react";
import type { ValueOption } from "../value-menu/value-menu";

// Native radios provide arrow-key navigation; equal grid cells keep the
// sliding selection indicator independent of text length.
export function SegmentedControl({
  label,
  value,
  options,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  options: ValueOption[];
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const name = useId();
  const index = Math.max(
    0,
    options.findIndex((option) => option.value === value),
  );
  return (
    <div
      className={`${styles.segmented_control} segmented-control`}
      role="radiogroup"
      aria-label={label}
      data-value={value}
      style={
        {
          "--segment-count": options.length,
          "--segment-index": index,
        } as CSSProperties
      }
    >
      <span className="segment-indicator" aria-hidden="true" />
      {options.map((option) => (
        <label
          key={option.value}
          className="segment"
          data-muted={!!option.muted}
          title={option.label}
        >
          <input
            type="radio"
            name={name}
            value={option.value}
            checked={value === option.value}
            disabled={disabled}
            onChange={() => onChange(option.value)}
          />
          <span>{option.label}</span>
        </label>
      ))}
    </div>
  );
}
