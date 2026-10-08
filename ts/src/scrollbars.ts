import { useEffect } from "react";

// Native scroll containers keep their own wheel, selection, undo and drag
// behavior. Only proximity state changes, without React renders or DOM wrappers.
const scrollSurfaces = [
  ".composer-editor textarea",
  ".card-body",
  ".question-cards",
  ".settings-form-pane",
  ".resource-view",
  ".file-explorer",
  ".file-tabs",
  ".setting-options",
  ".pinned-prompt > button",
  "pre",
  ".markdown table",
  ".monaco-editor",
  ".xterm",
].join(",");

export function useScrollbars() {
  useEffect(() => {
    const proximity = parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue(
        "--scroll-near-distance",
      ),
    );
    let active: HTMLElement | null = null;
    let frame = 0;
    let pointer: PointerEvent | undefined;
    const clear = () => {
      active?.removeAttribute("data-scrollbar-near-x");
      active?.removeAttribute("data-scrollbar-near-y");
      active = null;
    };
    const measure = () => {
      frame = 0;
      if (!pointer) return;
      let surface =
        pointer.target instanceof Element
          ? pointer.target.closest<HTMLElement>(scrollSurfaces)
          : null;
      while (
        surface &&
        !surface.matches(".monaco-editor, .xterm") &&
        surface.scrollHeight <= surface.clientHeight &&
        surface.scrollWidth <= surface.clientWidth
      )
        surface =
          surface.parentElement?.closest<HTMLElement>(scrollSurfaces) ?? null;
      if (surface !== active) clear();
      if (!surface?.isConnected) return;
      active = surface;
      const bounds = surface.getBoundingClientRect();
      const x = String(pointer.clientY >= bounds.bottom - proximity);
      const y = String(pointer.clientX >= bounds.right - proximity);
      if (surface.dataset.scrollbarNearX !== x)
        surface.dataset.scrollbarNearX = x;
      if (surface.dataset.scrollbarNearY !== y)
        surface.dataset.scrollbarNearY = y;
    };
    const move = (event: PointerEvent) => {
      if (event.pointerType === "touch") return;
      pointer = event;
      if (!frame) frame = requestAnimationFrame(measure);
    };
    const leave = () => {
      cancelAnimationFrame(frame);
      frame = 0;
      pointer = undefined;
      clear();
    };
    document.addEventListener("pointermove", move, { passive: true });
    document.documentElement.addEventListener("pointerleave", leave);
    return () => {
      leave();
      document.removeEventListener("pointermove", move);
      document.documentElement.removeEventListener("pointerleave", leave);
    };
  }, []);
}
