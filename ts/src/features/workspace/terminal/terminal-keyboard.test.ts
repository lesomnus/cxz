import { expect, it, vi } from "vitest";
import { installTerminalKeyboard } from "./terminal-keyboard";

function fixture() {
  let handler!: (event: KeyboardEvent) => boolean;
  const events = new Map<string, () => void>();
  let range:
    | { start: { x: number; y: number }; end: { x: number; y: number } }
    | undefined;
  const listen = (name: string) => (callback: () => void) => {
    events.set(name, callback);
    return { dispose: () => events.delete(name) };
  };
  const active = {
    type: "normal",
    cursorX: 4,
    cursorY: 1,
    baseY: 0,
    viewportY: 0,
    length: 8,
    getLine: (y: number) => ({
      getCell: (x: number) => ({
        getWidth: () => (y === 1 && x === 2 ? 2 : y === 1 && x === 3 ? 0 : 1),
      }),
    }),
  };
  const terminal = {
    cols: 8,
    rows: 3,
    buffer: { active, onBufferChange: listen("buffer") },
    attachCustomKeyEventHandler: (callback: typeof handler) => {
      handler = callback;
    },
    input: vi.fn(),
    scrollToLine: vi.fn(),
    onData: listen("data"),
    onResize: listen("resize"),
    onSelectionChange: listen("selection"),
    getSelectionPosition: () => range,
    clearSelection: vi.fn(() => {
      range = undefined;
      events.get("selection")?.();
    }),
    select: vi.fn((x: number, y: number, length: number) => {
      const end = y * terminal.cols + x + length;
      range = {
        start: { x, y },
        end: { x: end % terminal.cols, y: Math.floor(end / terminal.cols) },
      };
      events.get("selection")?.();
    }),
  };
  const dispose = installTerminalKeyboard(
    terminal as unknown as Parameters<typeof installTerminalKeyboard>[0],
  );
  function key(key: string, options: Partial<KeyboardEvent> = {}) {
    const event = {
      type: "keydown",
      key,
      code: key === "v" ? "KeyV" : key,
      ctrlKey: false,
      shiftKey: false,
      altKey: false,
      metaKey: false,
      isComposing: false,
      preventDefault: vi.fn(),
      ...options,
    } as unknown as KeyboardEvent;
    return { handled: !handler(event), event };
  }
  return { terminal, active, events, dispose, key };
}

it("lets native paste through without a clipboard read or control byte, and leaves composition alone", () => {
  const f = fixture();
  const paste = f.key("v", { ctrlKey: true });
  expect(paste.handled).toBe(true);
  expect(paste.event.preventDefault).not.toHaveBeenCalled();
  expect(f.terminal.input).not.toHaveBeenCalled();
  expect(f.key("v", { ctrlKey: true, isComposing: true }).handled).toBe(false);
  const erase = f.key("Backspace", { ctrlKey: true });
  expect(erase.event.preventDefault).toHaveBeenCalledOnce();
  f.key("Backspace", { ctrlKey: true, type: "keyup" });
  expect(f.terminal.input).toHaveBeenCalledExactlyOnceWith("\x17", true);
  f.dispose();
});

it("extends and shrinks selections through whole wide characters, including after collapsing the range", () => {
  const f = fixture();
  f.key("ArrowLeft", { shiftKey: true });
  expect(f.terminal.select).toHaveBeenLastCalledWith(2, 1, 2);
  f.key("ArrowLeft", { shiftKey: true });
  expect(f.terminal.select).toHaveBeenLastCalledWith(1, 1, 3);
  f.key("ArrowRight", { shiftKey: true });
  expect(f.terminal.select).toHaveBeenLastCalledWith(2, 1, 2);
  f.key("ArrowRight", { shiftKey: true });
  expect(f.terminal.getSelectionPosition()).toBeUndefined();
  f.key("ArrowLeft", { shiftKey: true });
  expect(f.terminal.select).toHaveBeenLastCalledWith(2, 1, 2);
  f.key("Escape");
  expect(f.terminal.getSelectionPosition()).toBeUndefined();
  expect(f.key("Escape").handled).toBe(false);
  f.dispose();
});

it("extends existing mouse selections using zero-based coordinates and follows the focus through scrollback", () => {
  const f = fixture();
  f.terminal.select(1, 0, 2);
  f.key("ArrowRight", { shiftKey: true });
  expect(f.terminal.select).toHaveBeenLastCalledWith(1, 0, 3);
  f.terminal.clearSelection();
  f.active.baseY = 5;
  f.active.viewportY = 5;
  f.active.cursorY = 2;
  for (let n = 0; n < 3; n++) f.key("ArrowUp", { shiftKey: true });
  expect(f.terminal.scrollToLine).toHaveBeenLastCalledWith(4);
  for (let n = 0; n < 10; n++) f.key("ArrowUp", { shiftKey: true });
  expect(f.terminal.select).toHaveBeenLastCalledWith(4, 0, 56);
  f.dispose();
});

it("preserves alternate-screen and modified arrow inputs and removes its handlers on disposal", () => {
  const f = fixture();
  expect(f.key("ArrowLeft", { shiftKey: true, ctrlKey: true }).handled).toBe(
    false,
  );
  f.active.type = "alternate";
  f.events.get("buffer")!();
  expect(f.key("ArrowLeft", { shiftKey: true }).handled).toBe(false);
  expect(f.terminal.select).not.toHaveBeenCalled();
  f.active.type = "normal";
  f.events.get("buffer")!();
  f.key("ArrowLeft", { shiftKey: true });
  f.events.get("resize")!();
  f.dispose();
  expect(f.events.size).toBe(0);
  expect(f.key("ArrowLeft", { shiftKey: true }).handled).toBe(false);
});
