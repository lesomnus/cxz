import { WorkResourcesMenu } from "./work-resources-menu";
import { MenuScrollArea, MenuItem, MenuItemLabel } from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Memory } from "#gen/cxz/memory_pb";
import { Id } from "@lesomnus/payday/pdid";
import type {
  WorkMemoryState,
  WorkMemorySource,
} from "#src/features/session/model/work-memories.ts";
import { MemoryVersionChanges } from "#src/features/memory/memory-version-changes.tsx";
import {
  initialMemoryVersion,
  useWorkMemoryVersions,
} from "#src/features/session/model/work-memory-versions.ts";
import menuStyles from "./created-memories-menu.module.css";
import { MenuLink } from "#src/shared/navigation/route-link.tsx";
import { memoryLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "#src/shared/components/anchored-menu.module.css";

function MemoryList({
  source,
  memories,
  state,
  close,
}: {
  source: WorkMemorySource;
  memories: Memory[];
  state: WorkMemoryState;
  close: () => void;
}) {
  const versions = useWorkMemoryVersions(source, memories);
  return (
    <MenuScrollArea
      viewportClassName="created-memories-menu-viewport"
      scrollbarLabel={t("Memory")}
    >
      {memories.map((memory) => (
        <MenuLink
          key={key(memory.id)}
          {...memoryLocation(source.project, new Id(memory.id).toString())}
          role="menuitem"
          title={memory.name || memory.alias}
          aria-description={memory.alias || undefined}
          onClick={close}
          trailing={
            <MemoryVersionChanges
              version={
                initialMemoryVersion(memory) ??
                (versions.data?.get(key(memory.id))?.revision ===
                memory.revision
                  ? versions.data?.get(key(memory.id))
                  : undefined)
              }
              className={menuStyles.changes}
            />
          }
        >
          <MenuItemLabel>{memory.name || memory.alias}</MenuItemLabel>
        </MenuLink>
      ))}
      {state.error && (
        <div className={styles.state} role="alert">
          <span>{state.error}</span>
          <MenuItem type="button" role="menuitem" onClick={state.refresh}>
            <MenuItemLabel>{t("Retry")}</MenuItemLabel>
          </MenuItem>
        </div>
      )}
      {!memories.length && !state.error && (
        <p className={styles.state} role="status">
          {t(
            state.loading
              ? "Loading…"
              : "No memories were created in this range.",
          )}
        </p>
      )}
      {memories.length > 0 && state.loading && (
        <p className={styles.note} role="status">
          {t("Loading…")}
        </p>
      )}
    </MenuScrollArea>
  );
}

export function CreatedMemoriesMenu({
  source,
  memories,
  state,
  ...props
}: {
  id: string;
  anchor: HTMLButtonElement;
  source: WorkMemorySource;
  memories: Memory[];
  state: WorkMemoryState;
  partial: boolean;
  close: (restoreFocus?: boolean) => void;
}) {
  useLocale();
  return (
    <WorkResourcesMenu {...props} label={t("Created memories")}>
      <MemoryList
        source={source}
        memories={memories}
        state={state}
        close={() => props.close()}
      />
    </WorkResourcesMenu>
  );
}
