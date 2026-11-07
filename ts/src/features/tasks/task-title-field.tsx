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
      htmlFor={id}
      fullWidth
      dirty={dirty}
      className={styles.titleField}
    >
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
    </FormField>
  );
}
