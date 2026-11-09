import { Button, SegmentedControl } from "@lesomnus/nox";
import { SourceDiff } from "@lesomnus/nox/editor";
import { useMemo } from "react";
import { MemoryReadFormat, type MemoryReadReply } from "#gen/cxz/memory_svc_pb";
import { Markdown } from "#src/shared/content/markdown.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { useTheme } from "#src/shared/theme/theme.tsx";
import { reverseMemoryDiff } from "./reverse-memory-diff";
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
        <MemoryChanges
          content={content}
          revision={revision}
          comparison={comparison}
        />
      )}
    </div>
  );
}

function MemoryChanges({
  content,
  revision,
  comparison,
}: {
  content: string;
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
    <MemoryDiff
      content={content}
      patch={changes.diff}
      retry={comparison.retry}
    />
  );
}

function MemoryDiff({
  content,
  patch,
  retry,
}: {
  content: string;
  patch: string;
  retry: () => void;
}) {
  const theme = useTheme();
  const settings = useEditorSettings();
  const previous = useMemo(() => {
    try {
      return { value: reverseMemoryDiff(content, patch) };
    } catch (error) {
      return { error: String(error) };
    }
  }, [content, patch]);
  if (previous.error)
    return (
      <div className={styles.failure}>
        <ErrorNotice local error={previous.error} />
        <Button onClick={retry}>Retry changes</Button>
      </div>
    );
  return (
    <div className={styles.diffViewport}>
      <SourceDiff
        path="memory.md"
        languageId="markdown"
        ariaLabel="Memory changes"
        original={previous.value!}
        modified={content}
        theme={theme}
        settings={{ ...settings, wordWrap: true }}
      />
    </div>
  );
}
