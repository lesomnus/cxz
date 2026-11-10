import { useCallback, useId, useState } from "react";
import { Button, MenuItem, MenuItemLabel } from "@lesomnus/nox";
import { AnchoredMenu } from "#src/shared/components/anchored-menu.tsx";
import {
  Icon,
  projectIconNames,
  type ProjectIconName,
} from "#src/shared/icons/icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./project-icon-picker.module.css";

// The glyphs are a set a person chooses from by looking, not a list they read,
// so they are laid out as a grid and each one says its own name underneath. A
// column of nineteen rows would make the choice into reading.
export function ProjectIconPicker({
  value,
  label,
  choose,
}: {
  value: ProjectIconName | undefined;
  /** Accessible name for the trigger, which carries no visible label. */
  label: string;
  choose: (name: ProjectIconName | undefined) => unknown;
}) {
  useLocale();
  const menuId = useId();
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [pending, setPending] = useState<string>();
  const [error, setError] = useState("");
  const close = useCallback(
    (restoreFocus = false) => {
      setAnchor(null);
      setError("");
      if (restoreFocus) anchor?.focus({ preventScroll: true });
    },
    [anchor],
  );
  // A rejected save keeps the menu open with its reason beside the choice. A
  // picker that closed on failure would report the new icon by showing the old
  // one, which reads as nothing having happened.
  async function pick(name: ProjectIconName | undefined) {
    if (pending) return;
    setPending(name ?? "");
    setError("");
    try {
      await choose(name);
      setAnchor(null);
      anchor?.focus({ preventScroll: true });
    } catch (e) {
      setError(String(e));
    } finally {
      setPending(undefined);
    }
  }
  const cell = (name: ProjectIconName | undefined, caption: string) => (
    <MenuItem
      key={name ?? ""}
      type="button"
      role="menuitemradio"
      aria-checked={value === name}
      aria-label={caption}
      className={styles.cell}
      disabled={!!pending && pending !== (name ?? "")}
      loading={pending === (name ?? "")}
      icon={<Icon name={name ?? "projects"} size={20} />}
      onClick={() => void pick(name)}
    >
      <MenuItemLabel className={styles.name}>{caption}</MenuItemLabel>
    </MenuItem>
  );
  return (
    <span className={styles.picker}>
      <Button
        type="button"
        className={styles.trigger}
        aria-label={value ? `${label}: ${value}` : label}
        aria-haspopup="menu"
        aria-expanded={!!anchor}
        aria-controls={anchor ? menuId : undefined}
        onClick={(event) => {
          const button = event.currentTarget;
          setAnchor((current) => (current ? null : button));
        }}
      >
        <Icon name={value ?? "projects"} />
      </Button>
      {anchor && (
        <AnchoredMenu
          id={menuId}
          anchor={anchor}
          label={label}
          close={close}
          className={styles.menu}
          footer={
            error ? (
              <p className={styles.error} role="alert">
                {error}
              </p>
            ) : undefined
          }
        >
          <div className={styles.grid} role="group" aria-label={label}>
            {cell(undefined, t("None"))}
            {projectIconNames.map((name) => cell(name, name))}
          </div>
        </AnchoredMenu>
      )}
    </span>
  );
}
