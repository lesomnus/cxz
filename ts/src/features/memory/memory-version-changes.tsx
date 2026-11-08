import { isFieldSet } from "@bufbuild/protobuf";
import { CompactNumber } from "@lesomnus/nox";
import {
  MemoryVersionSchema,
  type MemoryVersion,
} from "#gen/cxz/memory_svc_pb";
import styles from "./memory-library.module.css";

export function MemoryVersionChanges({
  version,
  id,
  className = "",
}: {
  version?: MemoryVersion;
  id?: string;
  className?: string;
}) {
  const classes = `${styles.versionStats} ${className}`.trim();
  if (version?.revision === 1n)
    return (
      <span id={id} className={classes}>
        Created
      </span>
    );
  const known =
    version &&
    isFieldSet(version, MemoryVersionSchema.field.linesAdded) &&
    isFieldSet(version, MemoryVersionSchema.field.linesRemoved);
  return (
    <span
      id={id}
      className={classes}
      aria-label={
        known
          ? `${version.linesAdded} lines added, ${version.linesRemoved} lines removed`
          : "Line changes unavailable"
      }
      title={
        known
          ? `Compared with r${version.revision - 1n}`
          : "The previous revision or its line counts are unavailable"
      }
    >
      {known ? (
        <>
          <span className={styles.versionChange}>
            +
            <CompactNumber
              value={version.linesAdded}
              label={`${version.linesAdded} lines added`}
            />
          </span>
          <span className={styles.versionChange}>
            −
            <CompactNumber
              value={version.linesRemoved}
              label={`${version.linesRemoved} lines removed`}
            />
          </span>
        </>
      ) : (
        "—"
      )}
    </span>
  );
}
