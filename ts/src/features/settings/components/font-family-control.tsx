import { useEffect, useRef, useState } from "react";
import { Button } from "@lesomnus/cxz-ui";
import { ValueMenu } from "@lesomnus/cxz-ui";
import {
  defaultEditorSettings,
  fontFamilyCSS,
  isGoogleFont,
  type FontFamilySetting,
} from "#src/shared/settings/editor-settings.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { loadGoogleFont } from "#src/shared/editor/google-fonts.ts";
import { useFontFamily } from "#src/shared/editor/use-font-family.ts";

const systemFont = defaultEditorSettings.fontFamily;
const presets = [systemFont, "monospace"];
function fontLabel(value: FontFamilySetting) {
  if (isGoogleFont(value)) return value.family;
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
  value?: FontFamilySetting;
  inherited: FontFamilySetting;
  disabled?: boolean;
  onChange: (value: FontFamilySetting | undefined) => void;
}) {
  const resolved = value ?? inherited;
  function modeFor(value?: FontFamilySetting) {
    return isGoogleFont(value)
      ? "google"
      : typeof value === "string" && !presets.includes(value)
        ? "custom"
        : (value ?? "");
  }
  const [mode, setMode] = useState(modeFor(value));
  const [draft, setDraft] = useState(fontFamilyCSS(resolved));
  const [google, setGoogle] = useState(
    isGoogleFont(resolved) ? resolved.family : "Roboto Mono",
  );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const request = useRef(0);
  const font = useFontFamily(
    fontFamilyCSS(resolved),
    isGoogleFont(resolved) ? resolved.family : undefined,
  );
  const savedKey = JSON.stringify([value, resolved]);
  useEffect(() => {
    request.current++;
    setLoading(false);
    setDraft(fontFamilyCSS(resolved));
    setGoogle(isGoogleFont(resolved) ? resolved.family : "Roboto Mono");
    setMode(modeFor(value));
    setError("");
  }, [savedKey]);
  useEffect(
    () => () => {
      request.current++;
    },
    [],
  );
  async function apply() {
    if (disabled) return;
    const token = ++request.current;
    if (mode === "google") {
      const next = { provider: "google" as const, family: google.trim() };
      if (!isGoogleFont(next)) {
        setError(t("Enter a Google Fonts family name."));
        return;
      }
      setError("");
      setLoading(true);
      try {
        await loadGoogleFont(next.family);
        if (token === request.current) onChange(next);
      } catch {
        if (token === request.current)
          setError(
            t(
              "Could not load the font. Check the name or connection and try again.",
            ),
          );
      } finally {
        if (token === request.current) setLoading(false);
      }
      return;
    }
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
        value={mode}
        disabled={disabled}
        options={[
          { value: "", label: fontLabel(inherited), muted: true },
          { value: systemFont, label: t("System monospace") },
          { value: "monospace", label: t("Browser monospace") },
          { value: "custom", label: t("Custom font") },
          { value: "google", label: t("Google Fonts") },
        ]}
        choose={(next) => {
          setError("");
          request.current++;
          setLoading(false);
          setMode(next);
          if (next !== "custom" && next !== "google")
            onChange(next || undefined);
        }}
      />
      {(mode === "custom" || mode === "google") && (
        <div className="font-family-custom">
          <input
            type="text"
            aria-label={
              mode === "google"
                ? t("{label} Google Fonts family", { label })
                : t("{label} custom font family", { label })
            }
            aria-invalid={!!error}
            value={mode === "google" ? google : draft}
            placeholder={mode === "google" ? "Roboto Mono" : undefined}
            disabled={disabled}
            spellCheck={false}
            onChange={(event) => {
              request.current++;
              setLoading(false);
              if (mode === "google") setGoogle(event.target.value);
              else setDraft(event.target.value);
              setError("");
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.nativeEvent.isComposing) {
                event.preventDefault();
                apply();
              }
            }}
          />
          <Button type="button" disabled={disabled || loading} onClick={apply}>
            {loading ? t("Loading font…") : t("Apply font")}
          </Button>
        </div>
      )}
      {mode === "google" && (
        <small className="muted">
          {t(
            "Enter a family from Google Fonts, such as Roboto Mono, JetBrains Mono or Noto Sans KR. Only the selected font is downloaded.",
          )}
        </small>
      )}
      {font.loading && <small role="status">{t("Loading font…")}</small>}
      {font.error && (
        <p role="alert">
          {t(
            "Could not load the font. Check the name or connection and try again.",
          )}{" "}
          <Button type="button" disabled={disabled} onClick={font.retry}>
            {t("Retry")}
          </Button>
        </p>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
