import { ValueMenu } from "@lesomnus/nox";
import {
  Icon,
  projectIconNames,
  type ProjectIconName,
} from "#src/shared/icons/icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./project-icon-picker.module.css";

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
  return (
    <div className={styles.picker}>
      <ValueMenu
        label={label}
        value={value ?? ""}
        valueIcon={null}
        variant="compact"
        columns={5}
        minMenuWidth={320}
        searchable
        awaitAction
        options={[
          { value: "", label: t("None"), icon: <Icon name="projects" /> },
          ...projectIconNames.map((name) => ({
            value: name,
            label: name,
            icon: <Icon name={name} />,
          })),
        ]}
        choose={(name) => choose(name ? (name as ProjectIconName) : undefined)}
      />
    </div>
  );
}
