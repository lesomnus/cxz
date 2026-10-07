import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { flushSync } from "react-dom";
import { Button } from "./button";
import { useFloatingCard } from "./floating-card";
import { indentEdit } from "./composer-indent";
import {
  codeBlocks,
  codeSyntax,
  codeSyntaxes,
  highlightCodeLines,
  resolvedCodeSyntax,
  type CodeBlock,
  type CodeToken,
} from "./composer-code";
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
  const [tabMovesFocus, setTabMovesFocus] = useState(false);
  const cursor = useRef<
    | { start: number; end: number; direction: "forward" | "backward" | "none" }
    | undefined
  >(undefined);
  const expectedEdit = useRef<string | undefined>(undefined);
  const ranges = pasteRanges(value, pastes);
  const latest = useRef({ value, replace });
  latest.current = { value, replace };
  const lines = value.split("\n");
  const blocks = useMemo(
    () =>
      codeBlocks(value).map((block) => {
        const syntax = resolvedCodeSyntax(block, value, pastes);
        return {
          ...block,
          detected: syntax,
          tokens: highlightCodeLines(
            value.slice(block.bodyStart, block.bodyEnd),
            syntax,
          ),
        };
      }),
    [value, pastes],
  );

  function syncScroll() {
    const el = input.current!;
    // Keep code backgrounds behind the native caret, while header controls and
    // chips can paint above the textarea without a transformed stacking context.
    mirror.current!.style.left = `${-el.scrollLeft}px`;
    mirror.current!.style.top = `${-el.scrollTop}px`;
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
      input.current!.setSelectionRange(
        cursor.current.start,
        cursor.current.end,
        cursor.current.direction,
      );
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
  function replace(
    start: number,
    end: number,
    text: string,
    caret?: number,
    selectionEnd?: number,
    direction: "forward" | "backward" | "none" = "none",
  ) {
    // Edit with native LF line breaks; untouched chips retain their original bytes.
    text = text.replaceAll("\r\n", "\n").replaceAll("\r", "\n");
    caret ??= start + text.length;
    selectionEnd ??= caret;
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
    cursor.current = { start: caret, end: selectionEnd, direction };
    el.setSelectionRange(caret, selectionEnd, direction);
    expectedEdit.current = undefined;
  }
  function changeSyntax(block: CodeBlock, syntax: string) {
    const header =
      block.indent + block.fence + (syntax === "auto" ? "" : syntax);
    const el = input.current!;
    const position = el.selectionStart;
    const delta = header.length - (block.headerEnd - block.start);
    replace(
      block.start,
      block.headerEnd,
      header,
      position > block.headerEnd ? position + delta : block.bodyStart + delta,
    );
  }
  function closeCode(block: CodeBlock) {
    if (!block.closed) {
      const suffix =
        (value.endsWith("\n") ? "" : "\n") + block.indent + block.fence + "\n";
      replace(value.length, value.length, suffix);
    } else if (value[block.end] !== "\n") {
      replace(block.end, block.end, "\n");
    } else {
      input.current!.focus({ preventScroll: true });
      input.current!.setSelectionRange(block.end + 1, block.end + 1);
    }
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
            aria-description={
              tabMovesFocus
                ? "Tab: 포커스 이동. Ctrl+M: 들여쓰기 모드로 전환."
                : "Tab: 2칸 들여쓰기. Shift+Tab: 내어쓰기. Ctrl+M: Tab으로 포커스 이동 전환."
            }
            placeholder={placeholder}
            value={value}
            rows={blocks.length ? Math.min(12, Math.max(5, lines.length)) : 3}
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
              if (
                event.ctrlKey &&
                !event.altKey &&
                !event.metaKey &&
                !event.shiftKey &&
                event.key.toLowerCase() === "m"
              ) {
                event.preventDefault();
                if (event.repeat) return;
                setTabMovesFocus(!tabMovesFocus);
                setNotice(
                  tabMovesFocus
                    ? "Tab: 들여쓰기 · Shift+Tab: 내어쓰기 · Ctrl+M: 포커스 이동 모드"
                    : "Tab: 포커스 이동 · Ctrl+M: 들여쓰기 모드",
                );
                return;
              }
              if (
                event.key === "Tab" &&
                !event.ctrlKey &&
                !event.altKey &&
                !event.metaKey &&
                !tabMovesFocus
              ) {
                event.preventDefault();
                const direction = el.selectionDirection;
                const selection = normalizedSelection();
                const edit = indentEdit(
                  value,
                  selection.start,
                  selection.end,
                  event.shiftKey,
                );
                if (value.slice(edit.from, edit.to) !== edit.text)
                  replace(
                    edit.from,
                    edit.to,
                    edit.text,
                    edit.start,
                    edit.end,
                    direction,
                  );
                return;
              }
              if (event.ctrlKey && event.key === "Enter") {
                event.preventDefault();
                if (canSend) el.form?.requestSubmit();
                return;
              }
              const selection = normalizedSelection();
              // Pair a completed opening fence and place the native cursor on
              // the empty body line between the visible Markdown delimiters.
              if (
                event.key === "`" &&
                !event.ctrlKey &&
                !event.metaKey &&
                !event.altKey &&
                selection.start === selection.end
              ) {
                const lineStart =
                  value.lastIndexOf("\n", selection.start - 1) + 1;
                const prefix = value.slice(lineStart, selection.start);
                const lineEnd = value.indexOf("\n", selection.start);
                if (
                  /^ {0,3}``$/.test(prefix) &&
                  selection.start === (lineEnd < 0 ? value.length : lineEnd) &&
                  !blocks.some((b) => lineStart > b.start && lineStart <= b.end)
                ) {
                  event.preventDefault();
                  const indent = prefix.slice(0, -2);
                  replace(
                    selection.start,
                    selection.end,
                    "`\n\n" + indent + "```",
                    selection.start + 2,
                  );
                  return;
                }
              }
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
                const block = blocks.find(
                  (b) => index >= b.startLine && index <= b.endLine,
                );
                const header = block?.startLine === index;
                const footer = block?.closed && block.endLine === index;
                const tokens =
                  block && !header && !footer
                    ? block.tokens[index - block.startLine - 1]
                    : undefined;
                function text(from: number, to: number) {
                  if (!tokens) return value.slice(from, to);
                  return tokenSpans(tokens, from - start, to - start);
                }
                const chips = ranges.filter(
                  (range) =>
                    range.start >= start && range.end <= start + line.length,
                );
                let offset = start;
                const parts = chips.flatMap((range) => {
                  const fragment = (
                    <span aria-hidden="true" key={`text-${range.start}`}>
                      {text(offset, range.start)}
                    </span>
                  );
                  offset = range.end;
                  return [
                    fragment,
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
                  <div
                    className={`editor-line${block ? " editor-code-line" : ""}${header ? " editor-code-header" : ""}${footer || (block && index === block.endLine) ? " editor-code-last" : ""}`}
                    key={index}
                  >
                    {header ? (
                      <>
                        <span aria-hidden="true" className="editor-code-fence">
                          {block!.indent + block!.fence}
                          <span className="editor-code-info">
                            {line.slice(
                              block!.indent.length + block!.fence.length,
                            )}
                          </span>
                        </span>
                        <div
                          className="editor-code-controls"
                          style={{
                            left: `${block!.indent.length + block!.fence.length}ch`,
                          }}
                        >
                          <select
                            aria-label={`Code syntax ${blocks.indexOf(block!) + 1}`}
                            value={codeSyntax(block!.syntax)}
                            onChange={(event) =>
                              changeSyntax(block!, event.target.value)
                            }
                          >
                            {codeSyntaxes.map((syntax) => (
                              <option key={syntax} value={syntax}>
                                {syntax === "auto"
                                  ? `Auto · ${block!.detected}`
                                  : syntax}
                              </option>
                            ))}
                            {!codeSyntaxes.includes(
                              codeSyntax(block!.syntax),
                            ) && (
                              <option value={codeSyntax(block!.syntax)}>
                                {block!.syntax}
                              </option>
                            )}
                          </select>
                          <Button
                            className="toolbar-button code-close"
                            type="button"
                            aria-label={`Close code block ${blocks.indexOf(block!) + 1}`}
                            title="코드 블록 끝내기"
                            onClick={() => closeCode(block!)}
                          >
                            ×
                          </Button>
                        </div>
                      </>
                    ) : (
                      <>
                        {parts}
                        <span aria-hidden="true">
                          {text(offset, start + line.length) ||
                            (line.length === 0 ? "\u200b" : "")}
                        </span>
                      </>
                    )}
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

function tokenSpans(tokens: CodeToken[], start: number, end: number) {
  let offset = 0;
  return tokens.map((token, index) => {
    const from = Math.max(0, start - offset);
    const to = Math.min(token.text.length, end - offset);
    offset += token.text.length;
    return to > from ? (
      <span key={index} className={token.className}>
        {token.text.slice(from, to)}
      </span>
    ) : null;
  });
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
