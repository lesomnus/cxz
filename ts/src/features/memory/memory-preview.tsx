import { Button, SegmentedControl } from "@lesomnus/nox";
import { MemoryReadFormat, type MemoryReadReply } from "#gen/cxz/memory_svc_pb";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import colors from "./memory-library.module.css";
import styles from "./memory-preview.module.css";

export type MemoryPreviewMode = "preview" | "changes";
export type MemoryComparison = {
  revision: bigint;
  data?: MemoryReadReply;
  loading: boolean;
  error: string;
  retry: () => void;
};

export function MemoryPreview({
  content,
  mode,
  changeMode,
  comparison,
}: {
  content: string;
  mode: MemoryPreviewMode;
  changeMode: (mode: MemoryPreviewMode) => void;
  comparison: MemoryComparison;
}) {
  const revision = comparison.revision;
  return (
    <div className={styles.preview}>
      <div className={styles.toolbar}>
        <span className={styles.caption}>
          {mode === "changes" && revision > 1n
            ? `r${revision - 1n} → r${revision}`
            : ""}
        </span>
        <SegmentedControl
          label="Memory view"
          size="compact"
          value={mode}
          options={[
            { value: "preview", label: "Preview" },
            { value: "changes", label: "Changes" },
          ]}
          onChange={(value) =>
            changeMode(value === "changes" ? "changes" : "preview")
          }
        />
      </div>
      {mode === "preview" ? (
        content.trim() ? (
          <Markdown text={content} />
        ) : (
          <p className={styles.notice}>This document is empty.</p>
        )
      ) : (
        <MemoryChanges revision={revision} comparison={comparison} />
      )}
    </div>
  );
}

function MemoryChanges({
  revision,
  comparison,
}: {
  revision: bigint;
  comparison: MemoryComparison;
}) {
  if (revision <= 1n)
    return (
      <p className={styles.notice}>
        This is the first revision. There is no earlier revision to compare.
      </p>
    );
  if (comparison.loading)
    return (
      <p className={styles.notice} role="status">
        Loading changes…
      </p>
    );
  if (comparison.error)
    return (
      <div className={styles.failure}>
        <ErrorNotice local error={comparison.error} />
        <Button onClick={comparison.retry}>Retry changes</Button>
      </div>
    );
  const changes = comparison.data;
  if (!changes || changes.format !== MemoryReadFormat.DIFF)
    return (
      <p className={styles.notice}>
        {changes?.fallbackReason === "base revision is not retained"
          ? "The previous revision is no longer retained."
          : "Changes are unavailable for this revision."}
      </p>
    );
  if (!changes.diff)
    return <p className={styles.notice}>No changes in this revision.</p>;
  return (
    <pre className={styles.diff} role="region" aria-label="Memory changes">
      <code>
        {changes.diff.split("\n").map((line, index, lines) => {
          const kind =
            (index === 0 && line.startsWith("--- ")) ||
            (index === 1 && line.startsWith("+++ ")) ||
            line.startsWith("@@") ||
            line.startsWith("\\")
              ? "meta"
              : line.startsWith("+")
                ? "added"
                : line.startsWith("-")
                  ? "removed"
                  : "context";
          return (
            <span
              key={index}
              data-diff-line={kind}
              className={`${styles.line} ${kind === "added" ? colors.versionAdded : kind === "removed" ? colors.versionRemoved : ""}`}
            >
              {line}
              {index < lines.length - 1 ? "\n" : ""}
            </span>
          );
        })}
      </code>
    </pre>
  );
}
