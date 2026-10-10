import { useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { Button } from "./button";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";

// A native modal keeps shortcuts and the background inert while the input
// occupies exactly the displayed value's bounds, even inside a floating card.
export function EditableValue({
  label,
  value,
  save,
  validate,
  monospace = false,
}: {
  label: string;
  value: string;
  save: (value: string) => Promise<void>;
  validate?: (value: string) => string | undefined;
  monospace?: boolean;
}) {
  useLocale();
  const trigger = useRef<HTMLButtonElement>(null);
  const [editing, setEditing] = useState(false);
  return (
    <>
      <Button
        ref={trigger}
        type="button"
        className="editable-value-trigger"
        data-monospace={monospace}
        aria-label={t("Edit {label}", { label })}
        aria-haspopup="dialog"
        onClick={() => setEditing(true)}
      >
        {value || "—"}
      </Button>
      {editing && (
        <ValueEditor
          label={label}
          value={value}
          origin={trigger.current!}
          save={save}
          validate={validate}
          close={() => setEditing(false)}
        />
      )}
    </>
  );
}

function ValueEditor({
  label,
  value,
  origin,
  save,
  validate,
  close,
}: {
  label: string;
  value: string;
  origin: HTMLButtonElement;
  save: (value: string) => Promise<void>;
  validate?: (value: string) => string | undefined;
  close: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const saving = useRef(false);
  const alive = useRef(false);
  const [draft, setDraft] = useState(value);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [position, setPosition] = useState<CSSProperties>();
  useLayoutEffect(() => {
    const node = dialog.current!;
    alive.current = true;
    const measure = () => {
      const bounds = origin.getBoundingClientRect();
      const font = getComputedStyle(origin);
      setPosition({
        left: bounds.left,
        top: bounds.top,
        width: bounds.width,
        font: font.font,
        letterSpacing: font.letterSpacing,
        "--editable-value-height": `${bounds.height}px`,
      } as CSSProperties);
    };
    measure();
    node.showModal();
    input.current!.focus({ preventScroll: true });
    input.current!.select();
    const observer = new ResizeObserver(measure);
    observer.observe(origin);
    window.addEventListener("resize", measure);
    // Keep the overlay anchored if its parent scrolls or the mobile keyboard opens.
    document.addEventListener("scroll", measure, true);
    window.visualViewport?.addEventListener("resize", measure);
    window.visualViewport?.addEventListener("scroll", measure);
    return () => {
      alive.current = false;
      observer.disconnect();
      window.removeEventListener("resize", measure);
      document.removeEventListener("scroll", measure, true);
      window.visualViewport?.removeEventListener("resize", measure);
      window.visualViewport?.removeEventListener("scroll", measure);
      node.close();
      if (origin.isConnected) origin.focus({ preventScroll: true });
    };
  }, [origin]);
  async function submit() {
    if (saving.current) return;
    const next = draft.trim();
    const invalid = validate?.(next);
    if (invalid) {
      setError(invalid);
      input.current?.focus({ preventScroll: true });
      return;
    }
    if (next === value) {
      close();
      return;
    }
    saving.current = true;
    setBusy(true);
    setError("");
    try {
      await save(next);
      if (alive.current) close();
    } catch (failure) {
      if (alive.current) setError(String(failure));
    } finally {
      saving.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return createPortal(
    <dialog
      ref={dialog}
      className="editable-value-dialog"
      style={position}
      aria-label={t("Edit {label}", { label })}
      aria-busy={busy}
      onCancel={(event) => {
        event.preventDefault();
        if (!saving.current) close();
      }}
      onKeyDown={(event) => {
        event.stopPropagation();
      }}
      onClick={(event) => {
        if (event.target !== event.currentTarget || saving.current) return;
        const box = event.currentTarget.getBoundingClientRect();
        if (
          event.clientX < box.left ||
          event.clientX > box.right ||
          event.clientY < box.top ||
          event.clientY > box.bottom
        )
          close();
      }}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          event.stopPropagation();
          void submit();
        }}
      >
        <input
          ref={input}
          aria-label={label}
          aria-invalid={!!error}
          aria-describedby={error ? "editable-value-error" : undefined}
          value={draft}
          readOnly={busy}
          onChange={(event) => {
            setDraft(event.target.value);
            setError("");
          }}
          autoComplete="off"
          spellCheck={false}
        />
        <div className="editable-value-overlay">
          {error && (
            <p id="editable-value-error" role="alert">
              {error}
            </p>
          )}
          <div className="editable-value-actions">
            <Button type="button" disabled={busy} onClick={close}>
              {t("Cancel")}
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? t("Working…") : t("Confirm")}
            </Button>
          </div>
        </div>
      </form>
    </dialog>,
    document.body,
  );
}
