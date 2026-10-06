import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type CSSProperties,
} from "react";
import { Button } from "./button";

type Sheet = {
  id: number;
  title: string;
  label?: string;
  content: (close: () => void) => ReactNode;
  origin: HTMLElement | null;
  closing?: boolean;
};
type SheetRequest = Pick<Sheet, "title" | "label" | "content">;
const OpenSheet = createContext<(sheet: SheetRequest) => void>(() => {});
const SheetStack = createContext<{
  sheets: Sheet[];
  close: (id: number) => void;
}>({ sheets: [], close: () => {} });

export function useBottomSheet() {
  return useContext(OpenSheet);
}

export function BottomSheetProvider({ children }: { children: ReactNode }) {
  const [sheets, setSheets] = useState<Sheet[]>([]);
  const current = useRef(sheets);
  current.current = sheets;
  const sequence = useRef(0);
  const timers = useRef(new Set<ReturnType<typeof setTimeout>>());
  useEffect(() => () => timers.current.forEach(clearTimeout), []);
  const open = useCallback((sheet: SheetRequest) => {
    const entry: Sheet = {
      ...sheet,
      id: ++sequence.current,
      origin: document.activeElement as HTMLElement | null,
    };
    // Keep the stack bounded even when browsing many events without closing.
    setSheets((old) => [
      ...old.filter((item) => !item.closing).slice(-5),
      entry,
    ]);
  }, []);
  const close = useCallback((id: number) => {
    const active = current.current.filter((sheet) => !sheet.closing);
    const top = active.at(-1);
    if (top?.id !== id) return;
    setSheets((old) =>
      old.map((sheet) =>
        sheet.id === id ? { ...sheet, closing: true } : sheet,
      ),
    );
    const previous = active.at(-2);
    const target = previous
      ? document.querySelector<HTMLElement>(
          `[data-sheet="${previous.id}"] .sheet-close`,
        )
      : top.origin?.isConnected
        ? top.origin
        : document.querySelector<HTMLElement>(".composer textarea");
    target?.focus({ preventScroll: true });
    const timer = setTimeout(() => {
      timers.current.delete(timer);
      setSheets((old) => old.filter((sheet) => sheet.id !== id));
    }, 240);
    timers.current.add(timer);
  }, []);
  useEffect(() => {
    if (!sheets.some((sheet) => !sheet.closing)) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.isComposing || event.defaultPrevented)
        return;
      event.preventDefault();
      close(current.current.filter((sheet) => !sheet.closing).at(-1)!.id);
    };
    document.addEventListener("keydown", escape);
    return () => document.removeEventListener("keydown", escape);
  }, [sheets, close]);
  const stack = useMemo(() => ({ sheets, close }), [sheets, close]);
  return (
    <OpenSheet.Provider value={open}>
      <SheetStack.Provider value={stack}>{children}</SheetStack.Provider>
    </OpenSheet.Provider>
  );
}

export function BottomSheetHost() {
  const { sheets, close } = useContext(SheetStack);
  const host = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const node = host.current!;
    const conversation = node.closest(".conversation")!;
    const header = conversation.querySelector("header")!;
    const measure = () => {
      const available =
        node.getBoundingClientRect().bottom -
        header.getBoundingClientRect().bottom -
        8;
      node.style.setProperty("--sheet-space", `${Math.max(0, available)}px`);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(conversation);
    observer.observe(node.parentElement!);
    observer.observe(header);
    observer.observe(conversation.querySelector(".transcript-area")!);
    window.addEventListener("resize", measure);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  const active = sheets.filter((sheet) => !sheet.closing);
  return (
    <div ref={host} className="bottom-sheet-host">
      {sheets.map((sheet) => (
        <SheetCard
          key={sheet.id}
          sheet={sheet}
          depth={active.length - 1 - active.indexOf(sheet)}
          close={() => close(sheet.id)}
        />
      ))}
    </div>
  );
}

function SheetCard({
  sheet,
  depth,
  close,
}: {
  sheet: Sheet;
  depth: number;
  close: () => void;
}) {
  const [entered, setEntered] = useState(false);
  const card = useRef<HTMLElement>(null);
  const active = depth === 0 && !sheet.closing;
  useLayoutEffect(() => {
    if (
      active &&
      entered &&
      (document.activeElement === document.body ||
        document.activeElement?.closest('[data-closing="true"]'))
    )
      card.current
        ?.querySelector<HTMLElement>(".sheet-close")
        ?.focus({ preventScroll: true });
  }, [active, entered]);
  useLayoutEffect(() => {
    let second = 0;
    const first = requestAnimationFrame(() => {
      second = requestAnimationFrame(() => {
        setEntered(true);
        if (card.current?.dataset.active === "true")
          card.current
            .querySelector<HTMLElement>(".sheet-close")
            ?.focus({ preventScroll: true });
      });
    });
    return () => {
      cancelAnimationFrame(first);
      cancelAnimationFrame(second);
    };
  }, []);
  return (
    <section
      ref={card}
      className="bottom-sheet"
      role="dialog"
      aria-label={sheet.label ?? sheet.title}
      aria-hidden={!active}
      inert={!active}
      data-sheet={sheet.id}
      data-active={active}
      data-entered={entered}
      data-closing={!!sheet.closing}
      style={
        {
          zIndex: sheet.id,
          "--sheet-depth": Math.min(2, depth),
        } as CSSProperties
      }
    >
      <header className="sheet-heading">
        <strong>{sheet.title}</strong>
        <Button
          className="sheet-close"
          type="button"
          aria-label={
            sheet.label === "붙여넣기 원문"
              ? "Close paste preview"
              : "Close details"
          }
          onClick={close}
        >
          ×
        </Button>
      </header>
      <div className="sheet-body">{sheet.content(close)}</div>
    </section>
  );
}
