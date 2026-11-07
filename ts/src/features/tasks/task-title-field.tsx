import { FormField, Popover } from "@lesomnus/nox";
import { useState } from "react";
import styles from "./task-detail-panels.module.css";

export function TaskTitleField({
  id,
  value,
  disabled,
  readOnly = false,
  dirty = false,
  change,
}: {
  id: string;
  value: string;
  disabled: boolean;
  readOnly?: boolean;
  dirty?: boolean;
  change: (value: string) => void;
}) {
  const [anchor, setAnchor] = useState<HTMLDivElement | null>(null);
  const [feedback, setFeedback] = useState("");
  const [copying, setCopying] = useState(false);
  async function copy() {
    if (copying) return;
    setCopying(true);
    try {
      await navigator.clipboard.writeText(value);
      setFeedback("Copied");
    } catch {
      setFeedback("Copy failed");
    } finally {
      setCopying(false);
    }
  }
  const selectAll = (node: HTMLElement) => {
    const range = document.createRange();
    range.selectNodeContents(node);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  };
  return (
    <FormField
      label="Title"
      htmlFor={readOnly ? undefined : id}
      fullWidth
      dirty={dirty}
      className={styles.titleField}
    >
      {readOnly ? (
        <>
          <div
            ref={setAnchor}
            id={id}
            className={styles.readOnlyTitle}
            role="textbox"
            aria-label="Title"
            aria-readonly="true"
            aria-multiline="true"
            aria-disabled={disabled || undefined}
            tabIndex={disabled ? -1 : 0}
            aria-haspopup="dialog"
            onClick={() => {
              if (!disabled) void copy();
            }}
            onKeyDown={(event) => {
              if (
                (event.ctrlKey || event.metaKey) &&
                !event.altKey &&
                !event.shiftKey &&
                event.key.toLowerCase() === "a"
              ) {
                event.preventDefault();
                selectAll(event.currentTarget);
              } else if (
                !disabled &&
                (event.key === "Enter" || event.key === " ")
              ) {
                event.preventDefault();
                void copy();
              }
            }}
          >
            <span className={styles.readOnlyTitleText}>{value}</span>
          </div>
          <Popover
            anchor={disabled ? null : anchor}
            placement="in-place"
            matchAnchorWidth
            interactive
            label="Task title"
            onOpenChange={(open) => {
              if (!open) setFeedback("");
            }}
            content={
              <span className={styles.expandedTitle}>
                <button
                  type="button"
                  className={styles.copyTitleText}
                  aria-label="Copy full title"
                  aria-busy={copying || undefined}
                  onClick={() => void copy()}
                  onKeyDown={(event) => {
                    if (
                      (event.ctrlKey || event.metaKey) &&
                      !event.altKey &&
                      !event.shiftKey &&
                      event.key.toLowerCase() === "a"
                    ) {
                      event.preventDefault();
                      selectAll(event.currentTarget);
                    }
                  }}
                >
                  {value}
                </button>
                {feedback && (
                  <span role="status" className={styles.copyFeedback}>
                    {feedback}
                  </span>
                )}
              </span>
            }
          />
        </>
      ) : (
        <>
          <textarea
            id={id}
            className={styles.titleInput}
            rows={2}
            required
            disabled={disabled}
            readOnly={readOnly}
            maxLength={256}
            value={value}
            onChange={(event) =>
              change(event.target.value.replace(/[\r\n]+/g, " "))
            }
            onKeyDown={(event) => {
              if (
                readOnly ||
                event.key !== "Enter" ||
                event.nativeEvent.isComposing
              )
                return;
              event.preventDefault();
              const form = event.currentTarget.form;
              const submit = form?.querySelector<HTMLButtonElement>(
                'button[type="submit"]',
              );
              if (submit && !submit.disabled) form?.requestSubmit(submit);
            }}
          />
          <span className={styles.titlePreview} aria-hidden="true">
            {value}
          </span>
        </>
      )}
    </FormField>
  );
}
