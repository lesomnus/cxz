import styles from "./floating-card.module.css";
import type { HTMLAttributes, ReactNode, Ref } from "react";
import { Button } from "../button/button";

export function FloatingCard({
  title,
  hideHeading = false,
  close,
  closeLabel,
  bodyTabIndex,
  footer,
  children,
  className = "",
  ref,
  ...props
}: Omit<HTMLAttributes<HTMLElement>, "title"> & {
  title: string;
  hideHeading?: boolean;
  close?: () => void;
  closeLabel?: string;
  bodyTabIndex?: number;
  footer?: ReactNode;
  ref?: Ref<HTMLElement>;
}) {
  return (
    <section
      ref={ref}
      className={`${styles.floating_card} floating-card ${className}`}
      {...props}
    >
      {!hideHeading && (
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
      )}
      <div className="card-body" tabIndex={bodyTabIndex}>
        {children}
      </div>
      {footer && <footer className="card-footer">{footer}</footer>}
    </section>
  );
}
