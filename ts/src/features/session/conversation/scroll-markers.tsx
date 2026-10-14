import { memo, type RefObject } from "react";
import styles from "./transcript-search.module.css";

export type ScrollTick = {
  id: string;
  kind: "input" | "assistant" | "search";
  top: number;
};

// Membership belongs to React. Per-frame positions are painted by Transcript
// through these refs, without rebuilding hundreds of JSX nodes on every tick.
export const ScrollMarkers = memo(
  function ScrollMarkers({
    markers,
    nodes,
  }: {
    markers: readonly ScrollTick[];
    nodes: RefObject<Map<string, HTMLSpanElement>>;
  }) {
    return (
      <div className="scroll-markers">
        {markers.map(({ id, kind, top }) => (
          <span
            key={id}
            ref={(node) => {
              if (!node) return;
              nodes.current.set(id, node);
              return () => {
                if (nodes.current.get(id) === node) nodes.current.delete(id);
              };
            }}
            className={`scroll-tick ${kind === "search" ? styles.marker : kind === "input" ? "scroll-marker" : "scroll-response-marker"}`}
            data-event={id}
            data-search={
              kind === "search" ? id.slice("search:".length) : undefined
            }
            data-prompt={kind === "input" ? id : undefined}
            data-response={kind === "assistant" ? id : undefined}
            style={{ top, opacity: kind === "search" ? 1 : undefined }}
            aria-hidden="true"
          />
        ))}
      </div>
    );
  },
  (before, after) =>
    before.nodes === after.nodes &&
    before.markers.length === after.markers.length &&
    before.markers.every(
      (marker, index) =>
        marker.id === after.markers[index].id &&
        marker.kind === after.markers[index].kind,
    ),
);
