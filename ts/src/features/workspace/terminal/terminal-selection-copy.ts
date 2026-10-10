import type { Terminal } from "@xterm/xterm";

// Copy completed selections, rather than every intermediate drag position.
// Selection changes from keyboard actions are synchronous with their gesture.
export function installTerminalSelectionCopy(
  terminal: Pick<Terminal, "getSelection" | "onSelectionChange">,
  screen: HTMLElement,
  enabled: () => boolean,
  notify: (result: "copied" | "failed") => void,
) {
  let dragging = false;
  let disposed = false;
  let attempt = 0;
  let previous = "";
  let pending = "";
  async function copy() {
    if (disposed || !enabled()) return;
    const text = terminal.getSelection();
    if (!text || text === previous || text === pending) return;
    const current = ++attempt;
    pending = text;
    try {
      await navigator.clipboard.writeText(text);
      if (disposed || current !== attempt || !enabled()) return;
      previous = text;
      notify("copied");
    } catch {
      if (!disposed && current === attempt && enabled()) notify("failed");
    } finally {
      if (current === attempt) pending = "";
    }
  }
  const begin = (event: PointerEvent) => {
    if (event.button === 0) dragging = true;
  };
  const finish = () => {
    if (!dragging) return;
    dragging = false;
    void copy();
  };
  const cancel = () => {
    dragging = false;
  };
  const selection = terminal.onSelectionChange(() => {
    if (!terminal.getSelection()) previous = "";
    if (!dragging) void copy();
  });
  screen.addEventListener("pointerdown", begin, true);
  // xterm publishes its final selection from document's mouseup handler.
  // Finish at window's bubbling phase so that notification stays suppressed
  // throughout that same gesture, including an immediately rejected write.
  window.addEventListener("mouseup", finish);
  screen.ownerDocument.addEventListener("pointercancel", cancel, true);
  window.addEventListener("blur", cancel);
  return () => {
    disposed = true;
    attempt++;
    selection.dispose();
    screen.removeEventListener("pointerdown", begin, true);
    window.removeEventListener("mouseup", finish);
    screen.ownerDocument.removeEventListener("pointercancel", cancel, true);
    window.removeEventListener("blur", cancel);
  };
}
