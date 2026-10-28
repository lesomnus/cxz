import {
  pasteRanges,
  type ComposerPaste,
  type PasteRange,
} from "./composer-pastes";
import { presentPaste, type PastePresentation } from "./paste-presentation";
import { fileChipText } from "@lesomnus/nox";

export type ProjectedPasteRange = PasteRange & {
  sourceStart: number;
  sourceEnd: number;
  presentation: PastePresentation;
};

// References live in the draft, never in the textarea's character layout.
// Position mapping preserves identity even when several chips look identical.
export class PasteProjection {
  readonly text: string;
  readonly ranges: ProjectedPasteRange[] = [];
  constructor(
    readonly source: string,
    readonly pastes: Map<string, ComposerPaste>,
    frozen?: ReadonlyMap<string, PastePresentation>,
  ) {
    let text = "",
      offset = 0;
    for (const range of pasteRanges(source, pastes)) {
      text += source.slice(offset, range.start);
      const presentation =
        frozen?.get(range.paste.token) ?? presentPaste(range.paste);
      const start = text.length;
      text += fileChipText(presentation.label);
      this.ranges.push({
        start,
        end: text.length,
        sourceStart: range.start,
        sourceEnd: range.end,
        paste: range.paste,
        presentation,
      });
      offset = range.end;
    }
    this.text = text + source.slice(offset);
  }
  toSource(position: number) {
    let delta = 0;
    for (const range of this.ranges) {
      if (position <= range.start) break;
      if (position < range.end) return range.sourceEnd;
      delta = range.sourceEnd - range.end;
    }
    return position + delta;
  }
  toDisplay(position: number) {
    let delta = 0;
    for (const range of this.ranges) {
      if (position <= range.sourceStart) break;
      if (position < range.sourceEnd) return range.end;
      delta = range.end - range.sourceEnd;
    }
    return position + delta;
  }
  splice(start: number, end: number, source: string) {
    return (
      this.source.slice(0, this.toSource(start)) +
      source +
      this.source.slice(this.toSource(end))
    );
  }
  edit(next: string, caret: number): string | undefined {
    let start = 0,
      oldEnd = this.text.length,
      newEnd = next.length;
    while (start < oldEnd && start < newEnd && this.text[start] === next[start])
      start++;
    while (
      oldEnd > start &&
      newEnd > start &&
      this.text[oldEnd - 1] === next[newEnd - 1]
    ) {
      oldEnd--;
      newEnd--;
    }
    // Equal adjacent labels can make a textual diff select the wrong chip.
    // The native caret anchors the edit; only accept it if both tails match.
    const inserted = newEnd - start,
      removed = oldEnd - start;
    const anchored = caret - inserted;
    if (
      anchored >= 0 &&
      anchored + removed <= this.text.length &&
      this.text.slice(0, anchored) === next.slice(0, anchored) &&
      this.text.slice(anchored + removed) === next.slice(anchored + inserted)
    ) {
      start = anchored;
      oldEnd = start + removed;
      newEnd = start + inserted;
    }
    if (
      this.ranges.some(
        ({ start: a, end: b }) =>
          (start < b && oldEnd > a && !(start <= a && oldEnd >= b)) ||
          (start === oldEnd && start > a && start < b),
      )
    )
      return undefined;
    return this.splice(start, oldEnd, next.slice(start, newEnd));
  }
}

// Native undo restores visible text. Keep bounded reference snapshots so it
// also restores the right bodies, rather than guessing from duplicate labels.
type EditState = {
  source: string;
  signature: string;
  views: Map<string, string>;
};
export class PasteEditHistory {
  private states: EditState[];
  private index = 0;
  private pastes: Map<string, ComposerPaste>;
  constructor(projection: PasteProjection) {
    this.pastes = projection.pastes;
    this.states = [
      {
        source: projection.source,
        signature: this.signature(projection.source),
        views: new Map([[projection.text, projection.source]]),
      },
    ];
  }
  private signature(source: string) {
    return pasteRanges(source, this.pastes)
      .map((range) => range.paste.token)
      .join("\0");
  }
  observe(projection: PasteProjection) {
    if (this.states[this.index].source === projection.source)
      this.addView(this.states[this.index], projection.source, projection.text);
  }
  private addView(state: EditState, source: string, view: string) {
    state.views.set(view, source);
    // Keep the original and recent views. Plain typing does not consume chip
    // identity history; native text edits can preserve references directly.
    if (state.views.size > 4) state.views.delete([...state.views.keys()][1]);
  }
  record(source: string, view: string) {
    const current = this.states[this.index];
    const signature = this.signature(source);
    if (current.signature === signature) {
      current.source = source;
      this.addView(current, source, view);
      return;
    }
    this.states.splice(this.index + 1);
    this.states.push({ source, signature, views: new Map([[view, source]]) });
    // Retain entry state as well as recent structural edits.
    if (this.states.length > 100) this.states.splice(1, 1);
    this.index = this.states.length - 1;
  }
  restore(view: string, direction: "historyUndo" | "historyRedo") {
    const current = this.states[this.index];
    const same = current.views.get(view);
    if (same !== undefined && same !== current.source) {
      current.source = same;
      return same;
    }
    const step = direction === "historyUndo" ? -1 : 1;
    for (
      let index = this.index + step;
      index >= 0 && index < this.states.length;
      index += step
    ) {
      const source = this.states[index].views.get(view);
      if (source !== undefined) {
        this.index = index;
        this.states[index].source = source;
        return source;
      }
    }
    return undefined;
  }
}
