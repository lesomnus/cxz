import { useEffect, useState } from "react";
import { Button } from "./button";
import { ValueMenu } from "./value-menu";
import { defaultEditorSettings } from "./editor-settings";
import { t } from "./i18n";

const systemFont = defaultEditorSettings.fontFamily;
const presets = [systemFont, "monospace"];
function fontLabel(value: string) {
  return value === systemFont
    ? t("System monospace")
    : value === "monospace"
      ? t("Browser monospace")
      : value;
}

export function FontFamilyControl({
  label,
  value,
  inherited,
  disabled,
  onChange,
}: {
  label: string;
  value?: string;
  inherited: string;
  disabled?: boolean;
  onChange: (value: string | undefined) => void;
}) {
  const resolved = value ?? inherited;
  const [custom, setCustom] = useState(
    value !== undefined && !presets.includes(value),
  );
  const [draft, setDraft] = useState(resolved);
  const [error, setError] = useState("");
  useEffect(() => {
    setDraft(resolved);
    setCustom(value !== undefined && !presets.includes(value));
    setError("");
  }, [value, resolved]);
  function apply() {
    if (disabled) return;
    const next = draft.trim();
    if (!next || next.length > 1024 || !CSS.supports("font-family", next)) {
      setError(t("Enter a valid CSS font family list."));
      return;
    }
    setError("");
    onChange(next);
  }
  return (
    <div className="font-family-control">
      <ValueMenu
        label={label}
        value={custom ? "custom" : (value ?? "")}
        disabled={disabled}
        options={[
          { value: "", label: fontLabel(inherited), muted: true },
          { value: systemFont, label: t("System monospace") },
          { value: "monospace", label: t("Browser monospace") },
          { value: "custom", label: t("Custom font") },
        ]}
        choose={(next) => {
          setError("");
          setCustom(next === "custom");
          if (next !== "custom") onChange(next || undefined);
        }}
      />
      {custom && (
        <div className="font-family-custom">
          <input
            type="text"
            aria-label={t("{label} custom font family", { label })}
            aria-invalid={!!error}
            value={draft}
            disabled={disabled}
            spellCheck={false}
            onChange={(event) => {
              setDraft(event.target.value);
              setError("");
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                event.preventDefault();
                apply();
              }
            }}
          />
          <Button type="button" disabled={disabled} onClick={apply}>
            {t("Apply font")}
          </Button>
        </div>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
