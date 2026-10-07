import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  Children,
  type HTMLAttributes,
  type Ref,
  type ReactNode,
} from "react";
import { Button } from "./button";

type Card = {
  id: number;
  title: string | (() => string);
  label?: string | (() => string);
  kind?: "paste";
  content: (close: () => void) => ReactNode;
  origin: HTMLElement | null;
  closing?: boolean;
  restoreFocus?: boolean;
};
type CardRequest = Pick<Card, "title" | "label" | "content" | "kind">;
const OpenCard = createContext<(card: CardRequest) => void>(() => {});
const CardState = createContext<{
  card?: Card;
  close: (id: number, restoreFocus?: boolean) => void;
}>({ close: () => {} });

export function useFloatingCard() {
  return useContext(OpenCard);
}

export function FloatingCardProvider({ children }: { children: ReactNode }) {
  useLocale();
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
    setCard({ ...active, closing: true, restoreFocus });
    timer.current = setTimeout(() => {
      setCard((old) => (old?.id === id ? undefined : old));
    }, 180);
  }, []);
  useEffect(() => {
    if (!card?.closing || card.restoreFocus === false) return;
    // A covered question becomes interactive again during this commit.
    const target = card.origin?.isConnected
      ? card.origin
      : document.querySelector<HTMLElement>(".composer textarea");
    target?.focus({ preventScroll: true });
  }, [card]);
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

export function FloatingCardHost({ children }: { children?: ReactNode }) {
  useLocale();
  const { card, close } = useContext(CardState);
  const host = useRef<HTMLDivElement>(null);
  const persistent = useRef<HTMLDivElement>(null);
  const hasQuestions = Children.count(children) > 0;
  useLayoutEffect(() => {
    const node = host.current!;
    const conversation = node.closest(".conversation")!;
    const header = conversation.querySelector("header")!;
    const wrapper = conversation.querySelector(".composer-wrapper")!;
    const measure = () => {
      const bounds = conversation.getBoundingClientRect();
      const anchor = wrapper.getBoundingClientRect();
      const inset = parseFloat(
        getComputedStyle(node).getPropertyValue("--card-shadow-space"),
      );
      const left = Math.min(inset, Math.max(0, anchor.left - bounds.left));
      const right = Math.min(inset, Math.max(0, bounds.right - anchor.right));
      node.style.setProperty("--card-shadow-left", `${left}px`);
      node.style.setProperty("--card-shadow-right", `${right}px`);
      node.style.left = `${anchor.left - bounds.left - left}px`;
      node.style.width = `${anchor.width + left + right}px`;
      node.style.bottom = `${bounds.bottom - anchor.top}px`;
      const available =
        node.getBoundingClientRect().bottom -
        parseFloat(getComputedStyle(node).paddingBottom) -
        header.getBoundingClientRect().bottom -
        8;
      node.style.setProperty("--card-space", `${Math.max(0, available)}px`);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(conversation);
    observer.observe(wrapper);
    observer.observe(header);
    observer.observe(conversation.querySelector(".transcript-area")!);
    window.addEventListener("resize", measure);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, []);
  useLayoutEffect(() => {
    const node = host.current!;
    const questions = persistent.current!;
    const preview = node.querySelector<HTMLElement>(":scope > .floating-card");
    const update = () => {
      const questionHeight = questions.offsetHeight;
      const previewHeight = preview?.offsetHeight ?? 0;
      const covered = !!card && !card.closing && questionHeight > 0;
      const lifted = covered && previewHeight >= questionHeight;
      const peek = parseFloat(getComputedStyle(node).paddingBottom) * 2 + 4;
      questions.dataset.covered = String(covered);
      questions.inert = covered;
      questions.setAttribute("aria-hidden", String(covered));
      questions.style.setProperty(
        "--question-lift",
        `${lifted ? 0.97 * questionHeight - previewHeight - peek : 0}px`,
      );
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(questions);
    if (preview) observer.observe(preview);
    return () => observer.disconnect();
  }, [card, hasQuestions]);
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
    <div
      ref={host}
      className="floating-card-host"
      data-questions={hasQuestions}
    >
      <div ref={persistent} className="question-cards" data-covered="false">
        {children}
      </div>
      {card && (
        <PreviewCard key={card.id} card={card} close={() => close(card.id)} />
      )}
    </div>
  );
}

function PreviewCard({ card, close }: { card: Card; close: () => void }) {
  useLocale();
  const title = typeof card.title === "function" ? card.title() : card.title;
  const label = typeof card.label === "function" ? card.label() : card.label;
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
    <FloatingCard
      ref={root}
      title={title}
      close={close}
      closeLabel={
        card.kind === "paste" ? t("Close paste preview") : t("Close details")
      }
      role="dialog"
      aria-label={label ?? title}
      aria-hidden={!active}
      inert={!active}
      data-card={card.id}
      data-active={active}
      data-entered={entered}
      data-closing={!!card.closing}
    >
      {card.content(close)}
    </FloatingCard>
  );
}

export function FloatingCard({
  title,
  close,
  closeLabel,
  children,
  className = "",
  ref,
  ...props
}: Omit<HTMLAttributes<HTMLElement>, "title"> & {
  title: string;
  close?: () => void;
  closeLabel?: string;
  ref?: Ref<HTMLElement>;
}) {
  useLocale();
  return (
    <section ref={ref} className={`floating-card ${className}`} {...props}>
      <header className="card-heading">
        <strong>{title}</strong>
        {close && (
          <Button
            className="card-close"
            type="button"
            aria-label={closeLabel}
            onClick={close}
          >
            ×
          </Button>
        )}
      </header>
      <div className="card-body">{children}</div>
    </section>
  );
}
