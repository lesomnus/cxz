import type { HTMLAttributes, Ref } from "react";
import { useLocale } from "./i18n-react";
import { Button } from "./button";

export function FloatingCard({
  title,
  close,
  closeLabel,
  bodyTabIndex,
  children,
  className = "",
  ref,
  ...props
}: Omit<HTMLAttributes<HTMLElement>, "title"> & {
  title: string;
  close?: () => void;
  closeLabel?: string;
  bodyTabIndex?: number;
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
      <div className="card-body" tabIndex={bodyTabIndex}>
        {children}
      </div>
    </section>
  );
}
