import {
  useLayoutEffect,
  useRef,
  type ButtonHTMLAttributes,
  type ReactNode,
  type Ref,
} from "react";

// Keep uniform scaling, but cap the longest edge's total contraction at 4px.
export function Button({
  children,
  pressTarget,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  pressTarget?: string;
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <button {...props}>
      <ButtonContent pressTarget={pressTarget}>{children}</ButtonContent>
    </button>
  );
}

export function ButtonContent({
  children,
  pressTarget,
}: {
  children: ReactNode;
  pressTarget?: string;
}) {
  const content = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    const el = pressTarget
      ? content.current?.querySelector<HTMLElement>(pressTarget)
      : content.current;
    if (!el) return;
    const resize = new ResizeObserver(([entry]) => {
      const longest = Math.max(
        entry.contentRect.width,
        entry.contentRect.height,
      );
      el.style.setProperty(
        "--press-scale",
        String(Math.max(0.95, 1 - 4 / (longest || 1))),
      );
    });
    resize.observe(el);
    return () => resize.disconnect();
  }, [pressTarget]);
  return (
    <span ref={content} className="button-content">
      {children}
    </span>
  );
}
