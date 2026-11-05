import { isFieldSet } from "@bufbuild/protobuf";
import {
  MemoryVersionSchema,
  type MemoryVersion,
} from "#gen/cxz/memory_svc_pb";
import styles from "./memory-library.module.css";

export function MemoryVersionChanges({
  version,
  id,
}: {
  version: MemoryVersion;
  id: string;
}) {
  if (version.revision === 1n)
    return (
      <span id={id} className={styles.versionStats}>
        Created
      </span>
    );
  const known =
    isFieldSet(version, MemoryVersionSchema.field.linesAdded) &&
    isFieldSet(version, MemoryVersionSchema.field.linesRemoved);
  return (
    <span
      id={id}
      className={styles.versionStats}
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
      {known ? `+${version.linesAdded} −${version.linesRemoved}` : "—"}
    </span>
  );
}
