import { Button } from "@lesomnus/cxz-ui";
import styles from "#src/shared/components/resource-surface.module.css";
export function QueuedMessage({
  text,
  busy,
  canRecall,
  recall,
  cancel,
}: {
  text: string;
  busy: boolean;
  canRecall: boolean;
  recall: () => void;
  cancel: () => void;
}) {
  if (!text) return null;
  return (
    <section className={styles.section} aria-label="Queued message">
      <div className={styles.row}>
        <strong>Waiting to send</strong>
        <Button
          disabled={busy || !canRecall}
          title={
            canRecall
              ? "Return to editor"
              : "Clear the current draft before recalling this message"
          }
          onClick={recall}
        >
          Recall
        </Button>
        <Button disabled={busy} onClick={cancel}>
          Cancel queued message
        </Button>
      </div>
      <p className={`${styles.status} ${styles.queuedText}`}>{text}</p>
    </section>
  );
}
