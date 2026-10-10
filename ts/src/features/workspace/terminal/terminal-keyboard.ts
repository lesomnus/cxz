import type { IBuffer, Terminal } from "@xterm/xterm";

type Point = { x: number; y: number };
type KeyboardTerminal = Pick<
  Terminal,
  | "attachCustomKeyEventHandler"
  | "buffer"
  | "cols"
  | "rows"
  | "input"
  | "getSelectionPosition"
  | "select"
  | "clearSelection"
  | "scrollToLine"
  | "onData"
  | "onResize"
  | "onSelectionChange"
>;

// Cell coordinates, rather than string offsets, keep wide/combined glyphs intact.
function boundary(buffer: IBuffer, point: Point) {
  while (
    point.x > 0 &&
    buffer.getLine(point.y)?.getCell(point.x)?.getWidth() === 0
  )
    point.x--;
  return point;
}

export function installTerminalKeyboard(terminal: KeyboardTerminal) {
  let selection: { anchor: Point; focus: Point; column: number } | undefined;
  let applying = false;
  const reset = () => {
    selection = undefined;
  };
  const changed = terminal.onSelectionChange(() => {
    if (!applying) reset();
  });
  const input = terminal.onData(reset);
  const resize = terminal.onResize(reset);
  const buffer = terminal.buffer.onBufferChange(reset);

  terminal.attachCustomKeyEventHandler((event) => {
    if (event.isComposing) return true;
    if (
      event.ctrlKey &&
      !event.altKey &&
      !event.metaKey &&
      event.code === "KeyV"
    ) {
      // Leave the browser's trusted paste gesture intact. xterm's paste event
      // handler handles clipboard text, line endings and bracketed-paste mode.
      return false;
    }
    if (
      event.ctrlKey &&
      !event.shiftKey &&
      !event.altKey &&
      !event.metaKey &&
      event.key === "Backspace"
    ) {
      event.preventDefault();
      if (event.type === "keydown") terminal.input("\x17", true); // ^W / word erase
      return false;
    }
    const active = terminal.buffer.active;
    if (active.type !== "normal") {
      reset();
      return true;
    }
    if (event.key === "Escape" && selection) {
      event.preventDefault();
      if (event.type === "keydown") {
        terminal.clearSelection();
        reset();
      }
      return false;
    }
    if (
      !event.shiftKey ||
      event.ctrlKey ||
      event.altKey ||
      event.metaKey ||
      !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)
    )
      return true;
    event.preventDefault();
    if (event.type !== "keydown") return false;
    if (!selection) {
      // xterm's getSelectionPosition returns zero-based cell coordinates.
      const existing = terminal.getSelectionPosition();
      const cursor = boundary(active, {
        x: active.cursorX,
        y: Math.max(
          active.viewportY,
          Math.min(
            active.baseY + active.cursorY,
            active.viewportY + terminal.rows - 1,
          ),
        ),
      });
      selection = {
        anchor: existing?.start ?? cursor,
        focus: existing?.end ?? { ...cursor },
        column: existing?.end.x ?? cursor.x,
      };
    }
    const { anchor, column } = selection;
    let focus = { ...selection.focus };
    if (event.key === "ArrowLeft") {
      if (focus.x > 0) focus.x--;
      else if (focus.y > 0) {
        focus.y--;
        focus.x = terminal.cols - 1;
      }
    } else if (event.key === "ArrowRight") {
      if (focus.x === terminal.cols && focus.y < active.length - 1) {
        focus.y++;
        focus.x = 0;
      }
      const width = active.getLine(focus.y)?.getCell(focus.x)?.getWidth() || 1;
      focus.x = Math.min(terminal.cols, focus.x + width);
    } else {
      focus.y = Math.max(
        0,
        Math.min(
          active.length - 1,
          focus.y + (event.key === "ArrowUp" ? -1 : 1),
        ),
      );
      focus.x = Math.min(column, terminal.cols);
    }
    focus = boundary(active, focus);
    selection.focus = focus;
    if (event.key === "ArrowLeft" || event.key === "ArrowRight")
      selection.column = focus.x;
    const origin = anchor.y * terminal.cols + anchor.x;
    const target = focus.y * terminal.cols + focus.x;
    const start = Math.min(origin, target);
    applying = true;
    try {
      if (origin === target) terminal.clearSelection();
      else
        terminal.select(
          start % terminal.cols,
          Math.floor(start / terminal.cols),
          Math.abs(target - origin),
        );
    } finally {
      applying = false;
    }
    if (focus.y < active.viewportY) terminal.scrollToLine(focus.y);
    else if (focus.y >= active.viewportY + terminal.rows)
      terminal.scrollToLine(focus.y - terminal.rows + 1);
    return false;
  });
  return () => {
    terminal.attachCustomKeyEventHandler(() => true);
    changed.dispose();
    input.dispose();
    resize.dispose();
    buffer.dispose();
  };
}
