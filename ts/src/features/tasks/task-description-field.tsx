import { useRef } from "react";
import { FormField, ToggleButton } from "@lesomnus/nox";
import { Icon } from "#src/shared/icons/icon.tsx";
import { TextareaControlCharacters } from "#src/shared/components/textarea-control-characters.tsx";
import styles from "./task-description-field.module.css";

export type TaskDescriptionOptions = {
  breakWords: boolean;
  showControls: boolean;
};
export const defaultDescriptionOptions: TaskDescriptionOptions = {
  breakWords: false,
  showControls: false,
};

export function TaskDescriptionField({
  id,
  value,
  disabled,
  readOnly = false,
  dirty = false,
  change,
  options,
  setOptions,
}: {
  id: string;
  value: string;
  disabled: boolean;
  readOnly?: boolean;
  dirty?: boolean;
  change: (value: string) => void;
  options: TaskDescriptionOptions;
  setOptions: (options: TaskDescriptionOptions) => void;
}) {
  const textarea = useRef<HTMLTextAreaElement>(null);
  return (
    <FormField
      label="Description"
      htmlFor={id}
      fullWidth
      dirty={dirty}
      className={styles.field}
      data-break-words={options.breakWords}
    >
      <div
        className={styles.tools}
        role="group"
        aria-label="Description display options"
      >
        <ToggleButton
          type="button"
          variant="toolbar"
          aria-label="Break words"
          title="Allow line breaks inside words"
          pressed={options.breakWords}
          onPressedChange={(breakWords) =>
            setOptions({ ...options, breakWords })
          }
        >
          <Icon name="word-wrap" />
        </ToggleButton>
        <ToggleButton
          type="button"
          variant="toolbar"
          aria-label="Show control characters"
          title="Show control characters"
          pressed={options.showControls}
          onPressedChange={(showControls) =>
            setOptions({ ...options, showControls })
          }
        >
          <Icon
            name={options.showControls ? "show-controls" : "hide-controls"}
          />
        </ToggleButton>
      </div>
      <textarea
        ref={textarea}
        id={id}
        wrap="soft"
        rows={5}
        disabled={disabled}
        readOnly={readOnly}
        maxLength={65536}
        value={value}
        onChange={(event) => change(event.target.value)}
      />
      {options.showControls && (
        <TextareaControlCharacters
          textarea={textarea}
          value={value}
          layoutKey={options.breakWords}
        />
      )}
    </FormField>
  );
}
