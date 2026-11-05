import { CopyButton } from "./copy-button";
import { useState } from "react";
import { Button, Popover } from "@lesomnus/nox";
import styles from "./identifier-value.module.css";

export function IdentifierValue({
  value,
  label,
  prefixLength = 7,
  copying = false,
  onCopy,
}: {
  value?: string;
  label?: string;
  prefixLength?: number;
  copying?: boolean;
  onCopy?: () => Promise<void>;
}) {
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [open, setOpen] = useState(false);
  if (!value) return <>—</>;
  const prefix = (
    <span className={styles.text}>{value.slice(0, prefixLength)}</span>
  );
  if (onCopy)
    return (
      <>
        <Button
          ref={setAnchor}
          type="button"
          effect="none"
          className={styles.copyValue}
          aria-label={label}
          aria-busy={copying || undefined}
          onClick={onCopy}
          onPointerEnter={(event) => {
            if (event.pointerType !== "touch") setOpen(true);
          }}
          onPointerLeave={() => {
            if (document.activeElement !== anchor) setOpen(false);
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
        >
          {prefix}
        </Button>
        <Popover
          anchor={anchor}
          open={open && value.length > prefixLength}
          onOpenChange={setOpen}
          placement="in-place"
          content={value}
        />
      </>
    );
  return (
    <span className={styles.value}>
      {value.length > prefixLength ? (
        <Popover content={value} placement="in-place">
          {prefix}
        </Popover>
      ) : (
        prefix
      )}
      <CopyButton value={value} />
    </span>
  );
}
