import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import {
  Button,
  Dialog,
  DialogBody,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  useUI,
} from "@lesomnus/nox";

/** cxz's action confirmation policy, composed from Nox's general Dialog parts. */
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
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const pending = useRef(false);
  const alive = useRef(false);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  useLayoutEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
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
  return (
    <Dialog
      ref={dialog}
      open
      onClose={close}
      dismissible={!busy}
      initialFocus={cancel}
      aria-busy={busy}
      className="confirmation-dialog"
    >
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <div>{children}</div>
        {failed && (
          <p role="alert">
            {failureMessage ?? t("Action failed. Please try again.")}
          </p>
        )}
      </DialogBody>
      <DialogFooter>
        <Button type="button" disabled={busy} onClick={() => void submit()}>
          {busy ? t("Working…") : confirmLabel}
        </Button>
        <Button ref={cancel} type="button" disabled={busy} onClick={close}>
          {t("Cancel")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
