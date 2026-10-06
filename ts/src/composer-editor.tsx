import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { Button } from "./button";
import { useFloatingCard } from "./floating-card";
import {
  createPaste,
  MAX_PASTE_BYTES,
  MAX_PASTE_CACHE_BYTES,
  needsPasteChip,
  partialPasteEdit,
  pasteRanges,
  wholePasteSelection,
  expandPastes,
  type ComposerPaste,
  type PasteRange,
} from "./composer-pastes";

export function ComposerEditor({
  value,
  pastes,
  onChange,
  canSend,
  ariaLabel = "Message",
  placeholder = "Continue the conversation…",
}: {
  value: string;
  pastes: Map<string, ComposerPaste>;
  onChange: (value: string) => void;
  canSend?: boolean;
  ariaLabel?: string;
  placeholder?: string;
}) {
  const input = useRef<HTMLTextAreaElement>(null);
  const mirror = useRef<HTMLDivElement>(null);
  const surface = useRef<HTMLDivElement>(null);
  const gutter = useRef<HTMLDivElement>(null);
  const openCard = useFloatingCard();
  const composing = useRef(false);
  const [composition, setComposition] = useState(false);
  const [notice, setNotice] = useState("");
  const cursor = useRef<number | undefined>(undefined);
  const expectedEdit = useRef<string | undefined>(undefined);
  const ranges = pasteRanges(value, pastes);
  const latest = useRef({ value, replace });
  latest.current = { value, replace };
  const lines = value.split("\n");

  function syncScroll() {
    const el = input.current!;
    mirror.current!.style.transform = `translate(${-el.scrollLeft}px, ${-el.scrollTop}px)`;
    gutter.current!.style.transform = `translateY(${-el.scrollTop}px)`;
  }
  function measure() {
    const el = input.current!;
    // Match the actual native text width, including a visible scrollbar's gutter.
    surface.current!.style.width = `${el.clientWidth}px`;
    const numbers = gutter.current!.children;
    Array.from(mirror.current!.children).forEach((line, index) => {
      (numbers[index] as HTMLElement).style.height =
        `${line.getBoundingClientRect().height}px`;
    });
    syncScroll();
  }
  useLayoutEffect(() => {
    measure();
    if (cursor.current !== undefined) {
      input.current!.setSelectionRange(cursor.current, cursor.current);
      cursor.current = undefined;
    }
  }, [value, composition]);
  useEffect(() => {
    const observer = new ResizeObserver(measure);
    observer.observe(input.current!);
    observer.observe(mirror.current!);
    return () => observer.disconnect();
  }, []);

  function normalizedSelection() {
    const el = input.current!;
    const selection = wholePasteSelection(
      ranges,
      el.selectionStart,
      el.selectionEnd,
    );
    if (
      selection.start !== el.selectionStart ||
      selection.end !== el.selectionEnd
    )
      el.setSelectionRange(selection.start, selection.end);
    return selection;
  }
  function replace(start: number, end: number, text: string) {
    // Edit with native LF line breaks; untouched chips retain their original bytes.
    text = text.replaceAll("\r\n", "\n").replaceAll("\r", "\n");
    const el = input.current!;
    const next = value.slice(0, start) + text + value.slice(end);
    expectedEdit.current = next;
    el.focus({ preventScroll: true });
    el.setSelectionRange(start, end);
    // Native insertion preserves the browser's undo stack and selection behavior.
    // setRangeText is the fallback for engines without insertText support.
    document.execCommand("insertText", false, text);
    if (el.value !== next) {
      el.setRangeText(text, start, end, "end");
      onChange(next);
    }
    cursor.current = start + text.length;
    expectedEdit.current = undefined;
  }
  function showPreview(range: PasteRange) {
    input.current!.focus({ preventScroll: true });
    input.current!.setSelectionRange(range.start, range.end);
    openCard({
      label: "붙여넣기 원문",
      title: `붙여넣기 · ${range.paste.lines}줄 · ${range.paste.bytes.toLocaleString()}B`,
      content: (close) => (
        <PastePreview
          paste={range.paste}
          apply={(text) => {
            // Cards are non-modal: editing behind a preview must never replace a
            // different occurrence at a stale offset.
            if (latest.current.value !== value) return false;
            // Release a background Question's inert state before native editing.
            flushSync(close);
            latest.current.replace(range.start, range.end, text);
            return true;
          }}
        />
      ),
    });
  }
  let lineOffset = 0;
  return (
    <>
      <div className="composer-editor" data-composing={composition}>
        <div
          className="editor-gutter"
          aria-hidden="true"
          style={{ width: `${String(lines.length).length + 2}ch` }}
        >
          <div ref={gutter}>
            {lines.map((_, index) => (
              <div key={index}>{index + 1}</div>
            ))}
          </div>
        </div>
        <div className="editor-body">
          <textarea
            ref={input}
            aria-label={ariaLabel}
            placeholder={placeholder}
            value={value}
            rows={3}
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
            wrap="soft"
            onScroll={syncScroll}
            onCompositionStart={() => {
              composing.current = true;
              setComposition(true);
            }}
            onCompositionEnd={() => {
              composing.current = false;
              setComposition(false);
            }}
            onSelect={() => {
              if (!composing.current) normalizedSelection();
            }}
            onBeforeInput={() => {
              if (!composing.current) normalizedSelection();
            }}
            onChange={(event) => {
              const next = event.target.value;
              if (
                !composing.current &&
                next !== expectedEdit.current &&
                partialPasteEdit(value, next, pastes)
              ) {
                event.target.value = value;
                normalizedSelection();
                return;
              }
              onChange(next);
            }}
            onPaste={(event) => {
              const text = event.clipboardData.getData("text/plain");
              if (!text) return;
              const selection = normalizedSelection();
              if (!needsPasteChip(text)) return;
              event.preventDefault();
              let paste = createPaste(
                text,
                crypto.randomUUID().replaceAll("-", "").slice(0, 8),
              );
              while (pastes.has(paste.token))
                paste = createPaste(
                  text,
                  crypto.randomUUID().replaceAll("-", "").slice(0, 8),
                );
              if (paste.bytes > MAX_PASTE_BYTES) {
                setNotice("붙여넣기는 1 MiB까지 가능합니다.");
                return;
              }
              if (
                paste.bytes +
                  [...pastes.values()].reduce((sum, p) => sum + p.bytes, 0) >
                MAX_PASTE_CACHE_BYTES
              ) {
                setNotice("붙여넣기 보관 공간이 32 MiB에 도달했습니다.");
                return;
              }
              pastes.set(paste.token, paste);
              setNotice("");
              replace(selection.start, selection.end, paste.token);
            }}
            onCopy={(event) => {
              const selection = normalizedSelection();
              if (selection.start === selection.end) return;
              event.preventDefault();
              event.clipboardData.setData(
                "text/plain",
                expandPastes(
                  value.slice(selection.start, selection.end),
                  pastes,
                ),
              );
            }}
            onCut={(event) => {
              const selection = normalizedSelection();
              if (selection.start === selection.end) return;
              event.preventDefault();
              event.clipboardData.setData(
                "text/plain",
                expandPastes(
                  value.slice(selection.start, selection.end),
                  pastes,
                ),
              );
              replace(selection.start, selection.end, "");
            }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || composing.current) return;
              const el = event.currentTarget;
              if (event.ctrlKey && event.key === "Enter") {
                event.preventDefault();
                if (canSend) el.form?.requestSubmit();
                return;
              }
              const selection = normalizedSelection();
              const chip = ranges.find(
                (range) =>
                  selection.start === range.start &&
                  selection.end === range.end,
              );
              if (
                (event.ctrlKey && event.key.toLowerCase() === "p") ||
                (event.key === "Enter" && chip)
              ) {
                event.preventDefault();
                const target = chip ?? ranges[0];
                if (target) {
                  showPreview(target);
                } else setNotice("현재 입력에 붙여넣기 chip이 없습니다.");
                return;
              }
              if (
                event.altKey ||
                event.ctrlKey ||
                event.metaKey ||
                event.shiftKey ||
                selection.start !== selection.end
              )
                return;
              const left =
                event.key === "ArrowLeft" || event.key === "Backspace";
              const right =
                event.key === "ArrowRight" || event.key === "Delete";
              const adjacent = ranges.find(
                (range) =>
                  (left && selection.start === range.end) ||
                  (right && selection.start === range.start),
              );
              if (!adjacent) return;
              if (event.key === "Backspace" || event.key === "Delete") {
                event.preventDefault();
                replace(adjacent.start, adjacent.end, "");
              } else if (left || right) {
                event.preventDefault();
                el.setSelectionRange(adjacent.start, adjacent.end);
              }
            }}
          />
          <div className="editor-surface" ref={surface}>
            <div className="editor-mirror" ref={mirror}>
              {lines.map((line, index) => {
                const start = lineOffset;
                lineOffset += line.length + 1;
                const chips = ranges.filter(
                  (range) =>
                    range.start >= start && range.end <= start + line.length,
                );
                let offset = start;
                const parts = chips.flatMap((range) => {
                  const text = (
                    <span aria-hidden="true" key={`text-${range.start}`}>
                      {value.slice(offset, range.start)}
                    </span>
                  );
                  offset = range.end;
                  return [
                    text,
                    <span
                      className="paste-chip"
                      role="button"
                      tabIndex={0}
                      aria-label={`붙여넣기 원문 보기: ${range.paste.lines}줄, ${range.paste.bytes} bytes`}
                      title="원문 보기 · Ctrl+P"
                      key={range.start}
                      onClick={() => showPreview(range)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          showPreview(range);
                        }
                      }}
                    >
                      {range.paste.token}
                    </span>,
                  ];
                });
                return (
                  <div className="editor-line" key={index}>
                    {parts}
                    <span aria-hidden="true">
                      {value.slice(offset, start + line.length) ||
                        (line.length === 0 ? "\u200b" : "")}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
      {notice && (
        <p className="editor-notice" role="status">
          {notice}
        </p>
      )}
    </>
  );
}

function PastePreview({
  paste,
  apply,
}: {
  paste: ComposerPaste;
  apply: (text: string) => boolean;
}) {
  const [stale, setStale] = useState(false);
  return (
    <>
      <pre>{paste.body}</pre>
      <div className="buttons">
        <Button
          type="button"
          disabled={stale}
          onClick={() => setStale(!apply(paste.body))}
        >
          원문 펼치기
        </Button>
        <Button
          type="button"
          disabled={stale}
          onClick={() => setStale(!apply(""))}
        >
          삭제
        </Button>
      </div>
      {stale && (
        <p role="status" className="muted">
          입력 내용이 변경되었습니다. chip을 다시 열어주세요.
        </p>
      )}
    </>
  );
}
