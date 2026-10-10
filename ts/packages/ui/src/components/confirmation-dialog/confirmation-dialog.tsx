import styles from "./confirmation-dialog.module.css";
import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Button } from "../button/button";
import { FloatingCard } from "../floating-card/floating-card";
import { useUI } from "../../providers/ui-provider";

// Native modal dialogs provide the focus boundary and make the rest of the
// page inert, including persistent questions and composer shortcuts.
export function ConfirmationDialog({
  title,
  children,
  confirmLabel,
  execute,
  close,
  failureMessage,
}: {
  title: string;
  children: ReactNode;
  confirmLabel: string;
  execute: () => Promise<boolean> | boolean;
  close: () => void;
  failureMessage?: string;
}) {
  const { t } = useUI();
  const id = useId();
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const alive = useRef(false);
  useLayoutEffect(() => {
    const node = dialog.current!;
    alive.current = true;
    node.showModal();
    cancel.current?.focus({ preventScroll: true });
    return () => {
      alive.current = false;
      node.close();
    };
  }, []);
  useLayoutEffect(() => {
    if (busy) dialog.current?.focus({ preventScroll: true });
  }, [busy]);
  async function submit() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setFailed(false);
    try {
      const done = await execute();
      if (!alive.current) return;
      if (done) close();
      else setFailed(true);
    } catch {
      if (alive.current) setFailed(true);
    } finally {
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return createPortal(
    <dialog
      ref={dialog}
      tabIndex={-1}
      className={`${styles.confirmation_dialog} confirmation-dialog`}
      aria-label={title}
      aria-describedby={id}
      aria-busy={busy}
      onCancel={(event) => {
        event.preventDefault();
        if (!pending.current) close();
      }}
      onKeyDown={(event) => {
        event.stopPropagation();
        if (event.key !== "Tab") return;
        const elements = [
          ...event.currentTarget.querySelectorAll<HTMLElement>(
            'button:not(:disabled), a[href], input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [tabindex]:not([tabindex="-1"])',
          ),
        ].filter(
          (element) =>
            element.getClientRects().length && !element.closest("[inert]"),
        );
        const first = elements[0],
          last = elements.at(-1);
        if (
          !first ||
          (event.shiftKey &&
            (document.activeElement === first ||
              document.activeElement === event.currentTarget)) ||
          (!event.shiftKey && document.activeElement === last)
        ) {
          event.preventDefault();
          (event.shiftKey ? last : first)?.focus({ preventScroll: true });
        }
      }}
      onClick={(event) => {
        if (event.target !== event.currentTarget || pending.current) return;
        const bounds = event.currentTarget.getBoundingClientRect();
        if (
          event.clientX < bounds.left ||
          event.clientX > bounds.right ||
          event.clientY < bounds.top ||
          event.clientY > bounds.bottom
        )
          close();
      }}
    >
      <FloatingCard
        title={title}
        data-entered="true"
        footer={
          <div className="confirmation-actions">
            <Button type="button" disabled={busy} onClick={() => void submit()}>
              {busy ? t("Working…") : confirmLabel}
            </Button>
            <Button ref={cancel} type="button" disabled={busy} onClick={close}>
              {t("Cancel")}
            </Button>
          </div>
        }
      >
        <div id={id}>{children}</div>
        {failed && (
          <p role="alert">
            {failureMessage ?? t("Action failed. Please try again.")}
          </p>
        )}
      </FloatingCard>
    </dialog>,
    document.body,
  );
}
