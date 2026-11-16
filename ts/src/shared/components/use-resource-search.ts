import { useCallback, useEffect, useRef, useState } from "react";

export function useResourceSearch() {
  const [open, setOpen] = useState(false);
  const area = useRef<HTMLElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const overlay = useRef<HTMLElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const origin = useRef<HTMLElement | null>(null);
  const opened = useRef(false);
  const show = useCallback(() => {
    if (!opened.current)
      origin.current = document.activeElement as HTMLElement | null;
    opened.current = true;
    setOpen(true);
    input.current?.focus({ preventScroll: true });
    input.current?.select();
  }, []);
  const close = useCallback(() => {
    opened.current = false;
    setOpen(false);
    const previous = origin.current;
    const target =
      previous?.isConnected &&
      previous !== document.body &&
      previous !== document.documentElement
        ? previous
        : trigger.current;
    target?.focus({ preventScroll: true });
  }, []);
  useEffect(() => {
    if (open) {
      input.current?.focus({ preventScroll: true });
      input.current?.select();
    }
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const pointerdown = (event: PointerEvent) => {
      const path = event.composedPath();
      if (
        (overlay.current && path.includes(overlay.current)) ||
        (trigger.current && path.includes(trigger.current))
      )
        return;
      opened.current = false;
      setOpen(false);
    };
    document.addEventListener("pointerdown", pointerdown, true);
    return () => document.removeEventListener("pointerdown", pointerdown, true);
  }, [open]);
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.isComposing || event.defaultPrevented || event.altKey) return;
      const target = event.target;
      if (!(target instanceof Element)) return;
      if (target.closest('dialog[open], [role="dialog"], [role="alertdialog"]'))
        return;
      if (
        target !== document.body &&
        target !== document.documentElement &&
        !area.current?.contains(target)
      )
        return;
      if (event.key === "Escape" && opened.current) {
        event.preventDefault();
        event.stopImmediatePropagation();
        close();
        return;
      }
      if (!event.shiftKey || event.key.toLowerCase() !== "f") return;
      if (
        !(event.ctrlKey || event.metaKey) &&
        target.closest(
          'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"], .monaco-editor',
        )
      )
        return;
      event.preventDefault();
      event.stopImmediatePropagation();
      show();
    };
    document.addEventListener("keydown", keydown, true);
    return () => document.removeEventListener("keydown", keydown, true);
  }, [show, close]);
  return { open, area, input, overlay, trigger, show, close };
}

export type ResourceSearchController = ReturnType<typeof useResourceSearch>;
