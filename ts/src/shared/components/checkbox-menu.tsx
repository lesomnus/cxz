import { useCallback, useId, useRef, useState, type ReactNode } from "react";
import {
  Button,
  MenuDivider,
  MenuItem,
  MenuItemLabel,
  MenuScrollArea,
} from "@lesomnus/nox";
import { AnchoredMenu } from "./anchored-menu";
import { CheckboxMark } from "./checkbox";
import styles from "./checkbox-menu.module.css";

export type CheckboxMenuOption = {
  value: string;
  label: string;
  icon?: ReactNode;
  disabled?: boolean;
};

/** Multiple-choice drafts commit once when the shared menu closes. */
export function CheckboxMenu({
  label,
  allLabel,
  options,
  value,
  choose,
  triggerContent,
}: {
  label: string;
  allLabel: string;
  options: readonly CheckboxMenuOption[];
  value: readonly string[];
  choose: (value: string[]) => void;
  triggerContent?: ReactNode;
}) {
  const id = useId();
  const button = useRef<HTMLButtonElement>(null);
  const [draft, setDraft] = useState<readonly string[] | null>(null);
  const pending = useRef<readonly string[] | null>(null);
  const latest = useRef({ value, choose });
  latest.current = { value, choose };
  const open = draft !== null;
  const chosen = draft ?? value;
  const edit = (next: readonly string[]) => {
    pending.current = next;
    setDraft(next);
  };
  const close = useCallback((restoreFocus = false) => {
    const next = pending.current;
    if (next === null) return;
    pending.current = null;
    setDraft(null);
    const { value, choose } = latest.current;
    if (
      next.length !== value.length ||
      next.some((item) => !value.includes(item))
    )
      choose([...next]);
    if (restoreFocus) button.current?.focus({ preventScroll: true });
  }, []);
  const enabled = options.filter((option) => !option.disabled);
  const all = enabled.every((option) => chosen.includes(option.value));
  const some = enabled.some((option) => chosen.includes(option.value));
  return (
    <>
      <Button
        ref={button}
        type="button"
        variant="toolbar"
        className={styles.trigger}
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        data-menu-open={open || undefined}
        data-filtered={!all || undefined}
        onClick={() => (open ? close() : edit([...value]))}
      >
        {triggerContent ?? label}
      </Button>
      {open && button.current && (
        <AnchoredMenu
          id={id}
          anchor={button.current}
          label={label}
          className={styles.menu}
          close={close}
        >
          <MenuScrollArea>
            <MenuItem
              role="menuitemcheckbox"
              aria-checked={all ? true : some ? "mixed" : false}
              disabled={!enabled.length}
              onClick={() =>
                edit(
                  all
                    ? chosen.filter(
                        (item) =>
                          !enabled.some((option) => option.value === item),
                      )
                    : [
                        ...new Set([
                          ...chosen,
                          ...enabled.map((option) => option.value),
                        ]),
                      ],
                )
              }
            >
              <CheckboxMark checked={all} indeterminate={!all && some} />
              <MenuItemLabel>{allLabel}</MenuItemLabel>
            </MenuItem>
            <MenuDivider />
            {options.map((option) => {
              const checked = chosen.includes(option.value);
              return (
                <MenuItem
                  key={option.value}
                  role="menuitemcheckbox"
                  aria-label={option.label}
                  aria-checked={checked}
                  disabled={option.disabled}
                  onClick={() =>
                    edit(
                      checked
                        ? chosen.filter((value) => value !== option.value)
                        : [...chosen, option.value],
                    )
                  }
                >
                  <CheckboxMark checked={checked} />
                  {option.icon && (
                    <span className={styles.optionIcon}>{option.icon}</span>
                  )}
                  <MenuItemLabel>{option.label}</MenuItemLabel>
                </MenuItem>
              );
            })}
          </MenuScrollArea>
        </AnchoredMenu>
      )}
    </>
  );
}
