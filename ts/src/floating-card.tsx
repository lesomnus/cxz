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
} from "react";
import { Button } from "./button";

type Card = {
  id: number;
  title: string;
  label?: string;
  content: (close: () => void) => ReactNode;
  origin: HTMLElement | null;
  closing?: boolean;
};
type CardRequest = Pick<Card, "title" | "label" | "content">;
const OpenCard = createContext<(card: CardRequest) => void>(() => {});
const CardState = createContext<{
  card?: Card;
  close: (id: number, restoreFocus?: boolean) => void;
}>({ close: () => {} });

export function useFloatingCard() {
  return useContext(OpenCard);
}

export function FloatingCardProvider({ children }: { children: ReactNode }) {
  const [card, setCard] = useState<Card>();
  const current = useRef(card);
  current.current = card;
  const sequence = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const open = useCallback((request: CardRequest) => {
    clearTimeout(timer.current);
    // A new action replaces the previous preview, with no retained back stack.
    setCard({
      ...request,
      id: ++sequence.current,
      origin: document.activeElement as HTMLElement | null,
    });
  }, []);
  const close = useCallback((id: number, restoreFocus = true) => {
    const active = current.current;
    if (!active || active.id !== id || active.closing) return;
    setCard({ ...active, closing: true });
    if (restoreFocus) {
      const target = active.origin?.isConnected
        ? active.origin
        : document.querySelector<HTMLElement>(".composer textarea");
      target?.focus({ preventScroll: true });
    }
    timer.current = setTimeout(() => {
      setCard((old) => (old?.id === id ? undefined : old));
    }, 180);
  }, []);
  useEffect(() => {
    if (!card || card.closing) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.isComposing || event.defaultPrevented)
        return;
      const active = current.current;
      if (!active || active.closing) return;
      event.preventDefault();
      close(active.id);
    };
    document.addEventListener("keydown", escape);
    return () => document.removeEventListener("keydown", escape);
  }, [card, close]);
  const state = useMemo(() => ({ card, close }), [card, close]);
  return (
    <OpenCard.Provider value={open}>
      <CardState.Provider value={state}>{children}</CardState.Provider>
    </OpenCard.Provider>
  );
}

export function FloatingCardHost() {
  const { card, close } = useContext(CardState);
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
      node.style.setProperty("--card-space", `${Math.max(0, available)}px`);
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
  useEffect(() => {
    if (!card || card.closing) return;
    const conversation = host.current!.closest(".conversation")!;
    const area = conversation.querySelector(".transcript-area")!;
    const column = conversation.querySelector(".transcript-content")!;
    const dismissInMargin = (event: PointerEvent) => {
      const target = event.target;
      if (
        event.button !== 0 ||
        !(target instanceof Element) ||
        !area.contains(target) ||
        target.closest(
          'button, a, input, textarea, select, [role="button"], [role="scrollbar"], .scroll-track, .pinned-prompt',
        )
      )
        return;
      const bounds = column.getBoundingClientRect();
      if (event.clientX < bounds.left || event.clientX > bounds.right)
        close(card.id, false);
    };
    document.addEventListener("pointerdown", dismissInMargin);
    return () => document.removeEventListener("pointerdown", dismissInMargin);
  }, [card, close]);
  return (
    <div ref={host} className="floating-card-host">
      {card && (
        <PreviewCard key={card.id} card={card} close={() => close(card.id)} />
      )}
    </div>
  );
}

function PreviewCard({ card, close }: { card: Card; close: () => void }) {
  const [entered, setEntered] = useState(false);
  const root = useRef<HTMLElement>(null);
  const active = !card.closing;
  useLayoutEffect(() => {
    let second = 0;
    const first = requestAnimationFrame(() => {
      second = requestAnimationFrame(() => {
        setEntered(true);
        if (root.current?.dataset.active === "true")
          root.current
            .querySelector<HTMLElement>(".card-close")
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
      ref={root}
      className="floating-card"
      role="dialog"
      aria-label={card.label ?? card.title}
      aria-hidden={!active}
      inert={!active}
      data-card={card.id}
      data-active={active}
      data-entered={entered}
      data-closing={!!card.closing}
    >
      <header className="card-heading">
        <strong>{card.title}</strong>
        <Button
          className="card-close"
          type="button"
          aria-label={
            card.label === "붙여넣기 원문"
              ? "Close paste preview"
              : "Close details"
          }
          onClick={close}
        >
          ×
        </Button>
      </header>
      <div className="card-body">{card.content(close)}</div>
    </section>
  );
}
