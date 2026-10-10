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
  type ReactNode,
  type RefObject,
  type Dispatch,
  type SetStateAction,
} from "react";
import { FloatingCard } from "./card-shell";
export { FloatingCard } from "./card-shell";
import { AnchoredDetail } from "./anchored-detail";

type Card = {
  id: number;
  title: string | (() => string);
  label?: string | (() => string);
  kind?: "paste";
  content: (close: () => void) => ReactNode;
  origin: HTMLElement | null;
  anchor?: HTMLElement;
  surface?: HTMLElement;
  closing?: boolean;
  restoreFocus?: boolean;
};
type CardRequest = Pick<
  Card,
  "title" | "label" | "content" | "kind" | "anchor" | "surface"
>;
const OpenCard = createContext<(card: CardRequest) => void>(() => {});
const CardState = createContext<{
  card?: Card;
  close: (id: number, restoreFocus?: boolean) => void;
}>({ close: () => {} });
const QuestionExpansion = createContext<{
  expanded?: string;
  setExpanded: Dispatch<SetStateAction<string | undefined>>;
}>({ setExpanded: () => {} });

export function useQuestionExpansion(id: string) {
  const { expanded, setExpanded } = useContext(QuestionExpansion);
  useEffect(
    () => () =>
      setExpanded((current) => (current === id ? undefined : current)),
    [id, setExpanded],
  );
  return {
    expanded: expanded === id,
    toggle: () => setExpanded((current) => (current === id ? undefined : id)),
  };
}

export function useFloatingCard() {
  return useContext(OpenCard);
}

export function useAnchoredCard(surface?: RefObject<HTMLElement | null>) {
  const anchor = useRef<HTMLButtonElement>(null);
  const closingClick = useRef(false);
  const open = useFloatingCard();
  const { card, close } = useContext(CardState);
  const present = !!card && card.anchor === anchor.current;
  const expanded = present && !card.closing;
  return {
    anchor,
    present,
    expanded,
    controls: expanded ? `event-details-${card.id}` : undefined,
    handlers(request: Omit<CardRequest, "anchor">) {
      return {
        onClick(event: React.MouseEvent<HTMLButtonElement>) {
          // Keyboard activation remains a single action. Pointer activation
          // opens on double click; a single click can close an existing card.
          if (event.detail <= 1) closingClick.current = expanded;
          if (expanded) close(card.id);
          else if (event.detail === 0 && anchor.current)
            open({
              ...request,
              anchor: anchor.current,
              surface: surface?.current ?? undefined,
            });
        },
        onDoubleClick() {
          // A double click on an open card must not reopen its closing preview.
          if (!expanded && !closingClick.current && anchor.current)
            open({
              ...request,
              anchor: anchor.current,
              surface: surface?.current ?? undefined,
            });
        },
      };
    },
  };
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
      origin: request.anchor ?? (document.activeElement as HTMLElement | null),
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
  const [expandedQuestion, setExpandedQuestion] = useState<string>();
  const expansion = useMemo(
    () => ({ expanded: expandedQuestion, setExpanded: setExpandedQuestion }),
    [expandedQuestion],
  );
  useLayoutEffect(() => {
    const node = host.current!;
    const conversation = node.closest<HTMLElement>(".conversation")!;
    const area = conversation.querySelector(".transcript-area")!;
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
        area.getBoundingClientRect().top -
        8;
      node.style.setProperty("--card-space", `${Math.max(0, available)}px`);
      node.style.setProperty("--question-max-height", `${bounds.height / 2}px`);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(conversation);
    observer.observe(wrapper);
    observer.observe(area);
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
      const covered =
        !!card && !card.anchor && !card.closing && questionHeight > 0;
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
    const node = host.current!;
    const questions = persistent.current!;
    const conversation = node.closest<HTMLElement>(".conversation")!;
    const area = conversation.querySelector<HTMLElement>(".transcript-area")!;
    node.dataset.retreated = "false";
    questions.dataset.retreated = "false";
    if (!hasQuestions) return;
    let reading = false;
    let near = false;
    let pointer: { x: number; y: number } | undefined;
    const update = () => {
      const focused = questions.contains(document.activeElement);
      const retreated = hasQuestions && reading && !near && !focused;
      node.dataset.retreated = String(retreated);
      questions.dataset.retreated = String(retreated);
    };
    const proximity = () => {
      if (!pointer) return false;
      const box = questions.getBoundingClientRect();
      return (
        pointer.x >= box.left - 32 &&
        pointer.x <= box.right + 32 &&
        pointer.y >= box.top - 32 &&
        pointer.y <= box.bottom + 32
      );
    };
    const scroll = () => {
      reading = true;
      near = proximity();
      update();
    };
    const move = (event: PointerEvent) => {
      pointer = { x: event.clientX, y: event.clientY };
      near = proximity();
      update();
    };
    const leave = () => {
      pointer = undefined;
      near = false;
      update();
    };
    const shadow = () => {
      const style = getComputedStyle(area);
      node.style.setProperty(
        "--question-fade-height",
        style.getPropertyValue("--bottom-fade-height") || "96px",
      );
    };
    update();
    shadow();
    const observer = new MutationObserver(shadow);
    observer.observe(area, { attributes: true, attributeFilter: ["style"] });
    conversation.addEventListener("conversation-reading-move", scroll);
    conversation.addEventListener("pointermove", move);
    conversation.addEventListener("pointerleave", leave);
    questions.addEventListener("focusin", update);
    questions.addEventListener("focusout", update);
    return () => {
      observer.disconnect();
      conversation.removeEventListener("conversation-reading-move", scroll);
      conversation.removeEventListener("pointermove", move);
      conversation.removeEventListener("pointerleave", leave);
      questions.removeEventListener("focusin", update);
      questions.removeEventListener("focusout", update);
    };
  }, [hasQuestions]);
  useEffect(() => {
    if (!expandedQuestion) return;
    const conversation = host.current!.closest(".conversation")!;
    const collapseInMargin = (event: PointerEvent) => {
      const target = event.target;
      if (
        event.button !== 0 ||
        !(target instanceof Element) ||
        !conversation.contains(target) ||
        target.closest(
          '.floating-card, button, a, input, textarea, select, [role="button"], [role="scrollbar"], .scroll-track',
        )
      )
        return;
      const question = persistent.current!.querySelector<HTMLElement>(
        '.question-card[data-expanded="true"]',
      );
      if (!question) return;
      const bounds = question.getBoundingClientRect();
      if (event.clientX < bounds.left || event.clientX > bounds.right)
        setExpandedQuestion(undefined);
    };
    document.addEventListener("pointerdown", collapseInMargin);
    return () => document.removeEventListener("pointerdown", collapseInMargin);
  }, [expandedQuestion]);
  useEffect(() => {
    if (!card || card.closing) return;
    const conversation = host.current!.closest(".conversation")!;
    const area = conversation.querySelector(".transcript-area")!;
    const column = conversation.querySelector(".transcript-content")!;
    const dismissInTranscript = (event: PointerEvent) => {
      const target = event.target;
      if (
        event.button !== 0 ||
        !(target instanceof Element) ||
        !area.contains(target)
      )
        return;
      if (card.anchor) {
        if (
          target.closest(".floating-card, .scroll-track, [role='scrollbar']") ||
          card.anchor.contains(target) ||
          card.surface?.contains(target)
        )
          return;
        close(card.id, false);
        return;
      }
      if (
        target.closest(
          'button, a, input, textarea, select, [role="button"], [role="scrollbar"], .scroll-track, .pinned-prompt',
        )
      )
        return;
      const bounds = column.getBoundingClientRect();
      if (event.clientX < bounds.left || event.clientX > bounds.right)
        close(card.id, false);
    };
    document.addEventListener("pointerdown", dismissInTranscript);
    return () =>
      document.removeEventListener("pointerdown", dismissInTranscript);
  }, [card, close]);
  return (
    <QuestionExpansion.Provider value={expansion}>
      <div
        ref={host}
        className="floating-card-host"
        data-questions={hasQuestions}
        data-expanded={!!expandedQuestion}
      >
        <div
          ref={persistent}
          className="question-cards"
          data-covered="false"
          data-expanded={!!expandedQuestion}
        >
          {children}
        </div>
        <div className="question-scroll-fade" aria-hidden="true" />
        {card &&
          (card.anchor ? (
            <AnchoredDetail
              key={card.id}
              anchor={card.anchor}
              surface={card.surface}
              id={`event-details-${card.id}`}
              title={
                typeof card.title === "function" ? card.title() : card.title
              }
              label={
                typeof card.label === "function" ? card.label() : card.label
              }
              closing={!!card.closing}
              close={(restore) => close(card.id, restore)}
            >
              {card.content(() => close(card.id))}
            </AnchoredDetail>
          ) : (
            <PreviewCard
              key={card.id}
              card={card}
              close={() => close(card.id)}
            />
          ))}
      </div>
    </QuestionExpansion.Provider>
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
