import {
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { Button, ResizeHandle } from "@lesomnus/nox";
import { Icon } from "#src/shared/icons/icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./workspace-editor-layer.module.css";

export function WorkspaceEditorLayer({
  children,
  close,
}: {
  children: ReactNode;
  close: () => void;
}) {
  useLocale();
  const layer = useRef<HTMLDivElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const [chosen, choose] = useState<number>();
  const [bounds, setBounds] = useState({ initial: 0, min: 0, max: 0 });
  useLayoutEffect(() => {
    const root = layer.current?.closest<HTMLElement>(".session-workspace");
    if (!root) return;
    const measure = () => {
      if (!layer.current) return;
      const css = getComputedStyle(layer.current);
      const gap =
        parseFloat(css.getPropertyValue("--workspace-preview-close-gap")) || 0;
      const max = Math.max(
        0,
        Math.floor(
          root.clientHeight -
            (closeButton.current?.offsetHeight ?? 0) -
            2 * gap,
        ),
      );
      const min = Math.min(
        max,
        parseFloat(
          css.getPropertyValue("--workspace-preview-drawer-min-height"),
        ) || 0,
      );
      const height = css
        .getPropertyValue("--workspace-preview-drawer-height")
        .trim();
      const initial = Math.max(
        min,
        Math.min(
          max,
          Math.round(
            parseFloat(height) *
              (height.endsWith("%") ? root.clientHeight / 100 : 1),
          ) || min,
        ),
      );
      setBounds((old) =>
        old.initial === initial && old.min === min && old.max === max
          ? old
          : { initial, min, max },
      );
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(root);
    if (closeButton.current) observer.observe(closeButton.current);
    return () => observer.disconnect();
  }, []);
  const height = Math.max(
    bounds.min,
    Math.min(bounds.max, chosen ?? bounds.initial),
  );
  return (
    <div ref={layer} className={`${styles.layer} workspace-editor-layer`}>
      <div
        className={styles.frame}
        data-workspace-preview-frame
        style={
          bounds.max > 0
            ? ({
                "--workspace-preview-resized-height": `${height}px`,
              } as CSSProperties)
            : undefined
        }
      >
        {children}
        <ResizeHandle
          axis="vertical"
          className={styles.resize}
          label={t("Resize file preview")}
          value={height}
          min={bounds.min}
          max={bounds.max}
          onChange={choose}
          onReset={() => choose(undefined)}
        />
        <Button
          ref={closeButton}
          className={styles.close}
          variant="toolbar"
          aria-label={t("Close file preview")}
          data-workspace-preview-close
          onClick={close}
        >
          <Icon name="close" />
        </Button>
      </div>
    </div>
  );
}
