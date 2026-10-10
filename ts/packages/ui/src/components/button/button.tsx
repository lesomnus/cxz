import styles from "./button.module.css";
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
  variant = "default",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  pressTarget?: string;
  variant?: "default" | "toolbar";
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <button
      {...props}
      data-variant={variant}
      className={`${styles.button} ${className}`}
    >
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
    <span ref={content} className={`${styles.button_content} button-content`}>
      {children}
    </span>
  );
}
