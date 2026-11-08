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
    isFieldSet(version, MemoryVersionSchema.field.bytesAdded) &&
    isFieldSet(version, MemoryVersionSchema.field.bytesRemoved);
  return (
    <span
      id={id}
      className={classes}
      aria-label={
        known
          ? `${version.bytesAdded} bytes added, ${version.bytesRemoved} bytes removed`
          : "Byte changes unavailable"
      }
      title={
        known
          ? `Compared with r${version.revision - 1n} (UTF-8 bytes)`
          : "The previous revision or its byte counts are unavailable"
      }
    >
      {known ? (
        <>
          <span className={`${styles.versionChange} ${styles.versionAdded}`}>
            +
            <CompactNumber
              value={Number(version.bytesAdded)}
              baseUnit="B"
              label={`${version.bytesAdded} bytes added`}
            />
          </span>
          <span className={styles.versionSeparator} aria-hidden="true">
            /
          </span>
          <span className={`${styles.versionChange} ${styles.versionRemoved}`}>
            −
            <CompactNumber
              value={Number(version.bytesRemoved)}
              baseUnit="B"
              label={`${version.bytesRemoved} bytes removed`}
            />
          </span>
        </>
      ) : (
        "—"
      )}
    </span>
  );
}
