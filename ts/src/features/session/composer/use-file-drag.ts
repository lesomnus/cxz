import { useEffect, useState } from "react";

export function useFileDrag(enabled: boolean) {
  const [dragging, setDragging] = useState(false);
  useEffect(() => {
    setDragging(false);
    if (!enabled) return;
    let depth = 0;
    const hasFiles = (event: DragEvent) =>
      event.dataTransfer?.types.includes("Files");
    const clear = () => {
      depth = 0;
      setDragging(false);
    };
    const enter = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      depth++;
      setDragging(true);
    };
    const over = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      // Prevent the browser opening a file outside the editor. Its own handler
      // changes the cursor to copy when the pointer reaches the drop target.
      event.preventDefault();
      event.dataTransfer!.dropEffect = "none";
      depth = Math.max(1, depth);
      setDragging(true);
    };
    const leave = (event: DragEvent) => {
      if (!hasFiles(event)) return;
      // Enter fires before leave when crossing descendants. Count both so
      // moving between the transcript, editor and its children never flashes.
      depth = Math.max(0, depth - 1);
      if (!depth) clear();
    };
    const drop = (event: DragEvent) => {
      if (hasFiles(event)) event.preventDefault();
      clear();
    };
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") clear();
    };
    window.addEventListener("dragenter", enter, true);
    window.addEventListener("dragover", over, true);
    window.addEventListener("dragleave", leave, true);
    window.addEventListener("drop", drop, true);
    window.addEventListener("dragend", clear, true);
    window.addEventListener("blur", clear);
    window.addEventListener("keydown", keydown, true);
    return () => {
      window.removeEventListener("dragenter", enter, true);
      window.removeEventListener("dragover", over, true);
      window.removeEventListener("dragleave", leave, true);
      window.removeEventListener("drop", drop, true);
      window.removeEventListener("dragend", clear, true);
      window.removeEventListener("blur", clear);
      window.removeEventListener("keydown", keydown, true);
    };
  }, [enabled]);
  return enabled && dragging;
}
