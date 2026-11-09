import { WorkResourcesMenu } from "./work-resources-menu";
import { MenuScrollArea, MenuItem, MenuItemLabel } from "@lesomnus/nox";
import { key } from "@lesomnus/payday/store";
import type { Project } from "#gen/cxz/project_pb";
import type { Memory } from "#gen/cxz/memory_pb";
import { Id } from "@lesomnus/payday/pdid";
import type { WorkMemoryState } from "#src/features/session/model/work-memories.ts";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { MenuLink } from "#src/shared/navigation/route-link.tsx";
import { memoryLocation } from "#src/shared/navigation/resource-route.ts";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "#src/shared/components/anchored-menu.module.css";

function MemoryList({
  project,
  memories,
  state,
  close,
}: {
  project: Project;
  memories: Memory[];
  state: WorkMemoryState;
  close: () => void;
}) {
  return (
    <MenuScrollArea
      viewportClassName="created-memories-menu-viewport"
      scrollbarLabel={t("Memory")}
    >
      {memories.map((memory) => (
        <MenuLink
          key={key(memory.id)}
          {...memoryLocation(project, new Id(memory.id).toString())}
          role="menuitem"
          icon={<ResourceIcon kind="memory" />}
          title={memory.name || memory.alias}
          aria-description={memory.alias || undefined}
          onClick={close}
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
  project,
  memories,
  state,
  ...props
}: {
  id: string;
  anchor: HTMLButtonElement;
  project: Project;
  memories: Memory[];
  state: WorkMemoryState;
  partial: boolean;
  close: (restoreFocus?: boolean) => void;
}) {
  useLocale();
  return (
    <WorkResourcesMenu {...props} label={t("Created memories")}>
      <MemoryList
        project={project}
        memories={memories}
        state={state}
        close={() => props.close()}
      />
    </WorkResourcesMenu>
  );
}
