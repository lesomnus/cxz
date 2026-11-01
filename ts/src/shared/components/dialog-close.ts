import { useRef, useState } from "react";

/** Let a conditionally mounted caller finish the Nox Dialog exit before cleanup. */
export function useDialogClose(close: () => void) {
  const [open, setOpen] = useState(true);
  const phase = useRef<"open" | "closing" | "closed">("open");
  const completion = useRef<(() => void) | undefined>(undefined);
  function request(afterClosed?: () => void) {
    if (phase.current !== "open") return;
    phase.current = "closing";
    completion.current = afterClosed;
    setOpen(false);
  }
  return {
    open,
    dismiss: () => request(),
    complete: (afterClosed: () => void) => request(afterClosed),
    onClosed: () => {
      if (phase.current !== "closing") return;
      phase.current = "closed";
      (completion.current ?? close)();
    },
  };
}
