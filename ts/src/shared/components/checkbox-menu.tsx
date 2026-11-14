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

/** A multiple-choice filter composed from the shared menu surface and rows. */
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
  const [open, setOpen] = useState(false);
  const close = useCallback((restoreFocus = false) => {
    setOpen(false);
    if (restoreFocus) button.current?.focus({ preventScroll: true });
  }, []);
  const enabled = options.filter((option) => !option.disabled);
  const all = enabled.every((option) => value.includes(option.value));
  const some = enabled.some((option) => value.includes(option.value));
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
        onClick={() => setOpen((value) => !value)}
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
                choose(
                  all
                    ? value.filter(
                        (item) =>
                          !enabled.some((option) => option.value === item),
                      )
                    : [
                        ...new Set([
                          ...value,
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
              const checked = value.includes(option.value);
              return (
                <MenuItem
                  key={option.value}
                  role="menuitemcheckbox"
                  aria-label={option.label}
                  aria-checked={checked}
                  disabled={option.disabled}
                  onClick={() =>
                    choose(
                      checked
                        ? value.filter((value) => value !== option.value)
                        : [...value, option.value],
                    )
                  }
                >
                  <CheckboxMark checked={checked} />
                  {option.icon}
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
