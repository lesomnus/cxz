import { useCallback, useEffect, useId, useRef, useState } from "react";
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
// so they are laid out as a grid and each one says its own name underneath.
// The menu covers the trigger: what it hides is the value it is about.
export function ProjectIconPicker({
  value,
  label,
  choose,
}: {
  value: ProjectIconName | undefined;
  label: string;
  choose: (name: ProjectIconName | undefined) => unknown;
}) {
  useLocale();
  const menuId = useId();
  const search = useRef<HTMLInputElement>(null);
  const [anchor, setAnchor] = useState<HTMLButtonElement | null>(null);
  const [query, setQuery] = useState("");
  const [pending, setPending] = useState<string>();
  const [error, setError] = useState("");
  const close = useCallback(
    (restoreFocus = false) => {
      setAnchor(null);
      setQuery("");
      setError("");
      if (restoreFocus) anchor?.focus({ preventScroll: true });
    },
    [anchor],
  );
  // The query is what a person came to type, so it takes the focus the menu
  // opens with; the arrow keys hand focus to the grid from there.
  useEffect(() => {
    if (anchor) search.current?.focus({ preventScroll: true });
  }, [anchor]);
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
      setQuery("");
      anchor?.focus({ preventScroll: true });
    } catch (e) {
      setError(String(e));
    } finally {
      setPending(undefined);
    }
  }
  const none = t("None");
  const matches = (caption: string) =>
    caption.toLowerCase().includes(query.trim().toLowerCase());
  const shown = projectIconNames.filter(matches);
  const emptyQuery = !shown.length && !matches(none);
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
        {/* Taller than the line it sits on, into the row's own padding, so the
            glyph reads as a glyph while the field keeps the height of the ones
            beside it. */}
        <Icon className={styles.glyph} name={value ?? "projects"} size={24} />
        <span className={styles.value}>{value ?? none}</span>
      </Button>
      {anchor && (
        <AnchoredMenu
          id={menuId}
          anchor={anchor}
          label={label}
          close={close}
          placement="overlay"
          autoFocus={false}
          className={styles.menu}
          header={
            <input
              ref={search}
              className={styles.search}
              type="search"
              value={query}
              placeholder={t("Search icons")}
              aria-label={t("Search icons")}
              onChange={(event) => setQuery(event.target.value)}
            />
          }
          footer={
            error ? (
              <p className={styles.error} role="alert">
                {error}
              </p>
            ) : undefined
          }
        >
          <div className={styles.grid} role="group" aria-label={label}>
            {matches(none) && cell(undefined, none)}
            {shown.map((name) => cell(name, name))}
          </div>
          {emptyQuery && (
            <p className={styles.empty} role="status">
              {t("No icons match.")}
            </p>
          )}
        </AnchoredMenu>
      )}
    </span>
  );
}
