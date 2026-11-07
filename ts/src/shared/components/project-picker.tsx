import { projectId } from "#src/shared/api/project-id.ts";
import { PrefixValueMenu } from "@lesomnus/nox";
import type { Project } from "#gen/cxz/project_pb";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./project-picker.module.css";

export function ProjectPicker({
  projects,
  value,
  loading = false,
  choose,
}: {
  projects: readonly Project[];
  value: string;
  loading?: boolean;
  choose: (value: string) => void;
}) {
  useLocale();
  const selected = projects.find((project) => projectId(project) === value);
  return (
    <div className={styles.picker}>
      <PrefixValueMenu
        className={styles.field}
        prefix={<ResourceIcon kind="projects" />}
        label={t("Select project")}
        value={value}
        display={
          selected
            ? selected.name || selected.alias
            : loading
              ? t("Loading projects…")
              : t("Select project")
        }
        options={projects.map((project) => ({
          value: projectId(project),
          label: project.name || project.alias,
          searchText: `${project.alias} ${project.workspace}`,
        }))}
        searchable
        disabled={!projects.length}
        choose={choose}
      />
    </div>
  );
}
