import { FormField } from "@lesomnus/nox";
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
  return (
    <FormField
      label="Title"
      htmlFor={readOnly ? undefined : id}
      fullWidth
      dirty={dirty}
      className={styles.titleField}
    >
      {readOnly ? (
        <div
          id={id}
          className={styles.readOnlyTitle}
          role="textbox"
          aria-label="Title"
          aria-readonly="true"
          aria-multiline="true"
          aria-disabled={disabled || undefined}
          tabIndex={disabled ? -1 : 0}
          onKeyDown={(event) => {
            if (
              (event.ctrlKey || event.metaKey) &&
              !event.altKey &&
              !event.shiftKey &&
              event.key.toLowerCase() === "a"
            ) {
              event.preventDefault();
              const range = document.createRange();
              range.selectNodeContents(event.currentTarget);
              const selection = window.getSelection();
              selection?.removeAllRanges();
              selection?.addRange(range);
            }
          }}
        >
          <span className={styles.readOnlyTitleText}>{value}</span>
        </div>
      ) : (
        <>
          <textarea
            id={id}
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
