import { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  Button,
  CompactNumber,
  Dialog,
  DialogBody,
  DialogFooter,
  SliderRoot,
  SliderControl,
  SliderValue,
} from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import styles from "./image-preview-dialog.module.css";

type Point = { x: number; y: number };
const center = { x: 0, y: 0 };
const clamp = (value: number, limit: number) =>
  Math.max(-limit, Math.min(limit, value));

/** One modal preview owns its Blob URL and read; closing releases both. */
export function ImagePreviewDialog({
  name,
  load,
  close,
}: {
  name: string;
  load: (signal: AbortSignal) => Promise<Blob>;
  close: () => void;
}) {
  useLocale();
  const viewport = useRef<HTMLDivElement>(null);
  const [box, setBox] = useState({ width: 0, height: 0 });
  const [natural, setNatural] = useState({ width: 0, height: 0 });
  const [url, setUrl] = useState("");
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [attempt, setAttempt] = useState(0);
  const [zoom, setZoom] = useState(100);
  const [pan, setPan] = useState<Point>(center);
  const [panning, setPanning] = useState(false);
  const drag = useRef<
    | {
        id: number;
        x: number;
        y: number;
        scaleX: number;
        scaleY: number;
        pan: Point;
      }
    | undefined
  >(undefined);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    let objectUrl = "";
    setUrl("");
    setNatural({ width: 0, height: 0 });
    setStatus("loading");
    setZoom(100);
    setPan(center);
    void Promise.resolve()
      .then(() => load(controller.signal))
      .then((blob) => {
        if (!active) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
      })
      .catch(() => {
        if (active) setStatus("error");
      });
    return () => {
      active = false;
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [load, attempt]);
  useLayoutEffect(() => {
    const node = viewport.current!;
    const measure = () =>
      setBox({ width: node.clientWidth, height: node.clientHeight });
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  const fit =
    natural.width && natural.height
      ? Math.min(box.width / natural.width, box.height / natural.height)
      : 0;
  const width = (natural.width * fit * zoom) / 100;
  const height = (natural.height * fit * zoom) / 100;
  const limitX = Math.max(0, (width - box.width) / 2);
  const limitY = Math.max(0, (height - box.height) / 2);
  const position = { x: clamp(pan.x, limitX), y: clamp(pan.y, limitY) };
  const canPan = status === "ready" && (limitX > 0 || limitY > 0);
  const endDrag = () => {
    drag.current = undefined;
    setPanning(false);
  };

  return (
    <Dialog
      open
      onClose={close}
      aria-label={t("Image preview")}
      className={styles.dialog}
    >
      <DialogBody
        ref={viewport}
        className={styles.viewport}
        data-image-viewport
        data-can-pan={canPan || undefined}
        data-panning={panning || undefined}
        role="region"
        aria-label={t("Image preview")}
        aria-description={t("Drag to move the image when zoomed in.")}
        aria-busy={status === "loading"}
        tabIndex={0}
        onPointerDown={(event) => {
          if (!canPan || event.button !== 0) return;
          event.preventDefault();
          const node = event.currentTarget;
          node.focus({ preventScroll: true });
          const bounds = node.getBoundingClientRect();
          drag.current = {
            id: event.pointerId,
            x: event.clientX,
            y: event.clientY,
            scaleX: bounds.width / node.clientWidth,
            scaleY: bounds.height / node.clientHeight,
            pan: position,
          };
          node.setPointerCapture(event.pointerId);
          setPanning(true);
        }}
        onPointerMove={(event) => {
          const start = drag.current;
          if (!start || start.id !== event.pointerId) return;
          setPan({
            x: clamp(
              start.pan.x + (event.clientX - start.x) / start.scaleX,
              limitX,
            ),
            y: clamp(
              start.pan.y + (event.clientY - start.y) / start.scaleY,
              limitY,
            ),
          });
        }}
        onPointerUp={(event) => {
          if (drag.current?.id !== event.pointerId) return;
          if (event.currentTarget.hasPointerCapture(event.pointerId))
            event.currentTarget.releasePointerCapture(event.pointerId);
          endDrag();
        }}
        onPointerCancel={endDrag}
        onLostPointerCapture={endDrag}
        onKeyDown={(event) => {
          if (!canPan || event.target !== event.currentTarget) return;
          const movement = {
            ArrowLeft: [-32, 0],
            ArrowRight: [32, 0],
            ArrowUp: [0, -32],
            ArrowDown: [0, 32],
          }[event.key];
          if (movement) {
            event.preventDefault();
            setPan({
              x: clamp(position.x + movement[0], limitX),
              y: clamp(position.y + movement[1], limitY),
            });
          } else if (event.key === "Home") {
            event.preventDefault();
            setPan(center);
          }
        }}
      >
        {url && (
          <img
            src={url}
            alt={name}
            draggable={false}
            className={styles.image}
            style={{
              width,
              height,
              transform: `translate(-50%, -50%) translate(${position.x}px, ${position.y}px)`,
              visibility: status === "ready" ? "visible" : "hidden",
            }}
            onLoad={(event) => {
              setNatural({
                width: event.currentTarget.naturalWidth,
                height: event.currentTarget.naturalHeight,
              });
              setStatus("ready");
            }}
            onError={() => setStatus("error")}
          />
        )}
        {status !== "ready" && (
          <div className={styles.status}>
            {status === "loading" ? (
              <p role="status">{t("Loading image…")}</p>
            ) : (
              <>
                <p role="alert">{t("The image could not be loaded.")}</p>
                <Button onClick={() => setAttempt((value) => value + 1)}>
                  {t("Retry")}
                </Button>
              </>
            )}
          </div>
        )}
      </DialogBody>
      <DialogFooter>
        <SliderRoot
          className={styles.zoom}
          label={t("Image zoom")}
          value={zoom}
          min={25}
          max={400}
          step={5}
          disabled={status !== "ready"}
          onChange={(next) => {
            endDrag();
            setZoom(next);
            setPan({
              x: clamp(
                position.x,
                Math.max(
                  0,
                  ((natural.width * fit * next) / 100 - box.width) / 2,
                ),
              ),
              y: clamp(
                position.y,
                Math.max(
                  0,
                  ((natural.height * fit * next) / 100 - box.height) / 2,
                ),
              ),
            });
          }}
        >
          <SliderControl aria-valuetext={`${zoom}%`} />
          <SliderValue className={styles.zoomValue}>
            <CompactNumber
              value={zoom}
              mode="integer"
              digits={3}
              label={`${zoom}%`}
            />
            %
          </SliderValue>
        </SliderRoot>
        <Button type="button" onClick={close}>
          {t("Close")}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
