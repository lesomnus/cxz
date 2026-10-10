import { useTheme } from "#src/shared/theme/theme.tsx";
import { t, translateKnown, currentLocale } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import {
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Ref,
} from "react";
import { flushSync } from "react-dom";
import { Button } from "@lesomnus/cxz-ui";
import { useFloatingCard } from "#src/features/session/cards/floating-card.tsx";
import { indentEdit } from "./composer-indent";
import { useCommandSuggestions } from "./command-suggestions";
import type { ComposerCommand } from "./composer-commands";
import {
  inlineBacktickEdit,
  inlineCodeRanges,
  listNewlineEdit,
} from "./composer-markdown";
import { useEditorSettings } from "#src/shared/settings/settings.ts";
import { paletteVariables } from "#src/shared/settings/editor-settings.ts";
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
  createFileChip,
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
import { droppedFiles, pickedFiles, type PickedFile } from "./composer-files";
import { MAX_UPLOAD_BYTES, type UploadFile } from "./composer-upload";

export type ComposerEditorHandle = {
  insertFiles: (files: readonly File[]) => void;
};

export function ComposerEditor({
  value,
  pastes,
  onChange,
  readOnly = false,
  canSend,
  commands,
  ariaLabel = t("Message"),
  placeholder = t("Continue the conversation…"),
  editorRef,
  upload,
  onUploadChange,
}: {
  value: string;
  pastes: Map<string, ComposerPaste>;
  onChange: (value: string) => void;
  readOnly?: boolean;
  canSend?: boolean;
  commands?: readonly ComposerCommand[];
  ariaLabel?: string;
  placeholder?: string;
  editorRef?: Ref<ComposerEditorHandle>;
  upload?: UploadFile;
  onUploadChange?: () => void;
}) {
  useLocale();
  const input = useRef<HTMLTextAreaElement>(null);
  const theme = useTheme();
  const editorSettings = useEditorSettings("session");
  const mirror = useRef<HTMLDivElement>(null);
  const surface = useRef<HTMLDivElement>(null);
  const gutter = useRef<HTMLDivElement>(null);
  const openCard = useFloatingCard();
  const composing = useRef(false);
  const [composition, setComposition] = useState(false);
  const [notice, setNotice] = useState("");
  const [dragging, setDragging] = useState(false);
  const [, refreshUploads] = useState(0);
  const jobs = useRef(new Map<string, AbortController>());
  const queue = useRef(Promise.resolve());
  const mounted = useRef(true);
  const notify = useRef(onUploadChange);
  notify.current = onUploadChange;
  const [tabMovesFocus, setTabMovesFocus] = useState(false);
  const cursor = useRef<
    | { start: number; end: number; direction: "forward" | "backward" | "none" }
    | undefined
  >(undefined);
  const expectedEdit = useRef<string | undefined>(undefined);
  const followOnEdit = useRef(false);
  const ranges = pasteRanges(value, pastes);
  const latest = useRef({ value, replace, readOnly });
  latest.current = { value, replace, readOnly };
  function changedUploads(paste: ComposerPaste) {
    paste.attachment?.listeners?.forEach((listener) => listener());
    if (!mounted.current) return;
    refreshUploads((v) => v + 1);
    notify.current?.();
  }
  function startUpload(paste: ComposerPaste) {
    if (!upload || !paste.attachment?.file || jobs.current.has(paste.token))
      return;
    const attachment = paste.attachment;
    const controller = new AbortController();
    jobs.current.set(paste.token, controller);
    attachment.state = "uploading";
    attachment.error = undefined;
    changedUploads(paste);
    const uploader = upload;
    queue.current = queue.current.then(async () => {
      try {
        controller.signal.throwIfAborted();
        const path = await uploader(
          attachment.file!,
          AbortSignal.any([controller.signal, AbortSignal.timeout(600_000)]),
        );
        controller.signal.throwIfAborted();
        attachment.path = path;
        attachment.state = "ready";
      } catch (error) {
        attachment.state = "error";
        attachment.error = controller.signal.aborted
          ? t("Upload cancelled.")
          : String(error instanceof Error ? error.message : error);
      } finally {
        jobs.current.delete(paste.token);
        changedUploads(paste);
      }
    });
  }
  async function insertPicked(picking: Promise<PickedFile[]> | PickedFile[]) {
    try {
      const picked = await picking;
      if (!mounted.current || latest.current.readOnly || !upload) return;
      const chips = picked.map(({ file, name, directory }) => {
        if (file.size > MAX_UPLOAD_BYTES)
          throw new Error(t("Files are limited to 1 GiB."));
        return createFileChip(file, name, directory);
      });
      if (!chips.length) return;
      const el = input.current!;
      const selection = wholePasteSelection(
        pasteRanges(latest.current.value, pastes),
        el.selectionStart,
        el.selectionEnd,
      );
      for (const chip of chips) pastes.set(chip.token, chip);
      latest.current.replace(
        selection.start,
        selection.end,
        chips.map((chip) => chip.token).join("\n"),
      );
      setNotice("");
      for (const chip of chips) startUpload(chip);
    } catch (error) {
      if (mounted.current)
        setNotice(String(error instanceof Error ? error.message : error));
    }
  }
  useImperativeHandle(editorRef, () => ({
    insertFiles: (files) => {
      try {
        void insertPicked(pickedFiles(files));
      } catch (error) {
        setNotice(String(error instanceof Error ? error.message : error));
      }
    },
  }));
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      for (const job of jobs.current.values()) job.abort();
    };
  }, []);
  useEffect(() => {
    for (const [token, controller] of jobs.current)
      if (!value.includes(token)) controller.abort();
  }, [value]);
  const lines = value.split("\n");
  const commandHints = useCommandSuggestions({
    value,
    commands: readOnly ? undefined : commands,
    input,
    composing: composition,
    accept: (name, end) => replace(0, end, name),
  });
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
  // Plain drafts use the native text painter, including throughout IME edits.
  // Only decorations need the mirrored text layer over the native caret.
  const decorated =
    commandHints.open ||
    blocks.length > 0 ||
    ranges.length > 0 ||
    lines.some((line) => inlineCodeRanges(line).length > 0);

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
    const style = getComputedStyle(el);
    const baseline = parseFloat(
      style.getPropertyValue("--composer-editor-height"),
    );
    // These sizes become CSS lengths; screen-space bounds would apply zoom twice.
    const contentHeight =
      parseFloat(getComputedStyle(mirror.current!).height) +
      parseFloat(style.paddingTop) +
      parseFloat(style.paddingBottom);
    const expanded = String(contentHeight > baseline + 1);
    if (el.dataset.expanded !== expanded) el.dataset.expanded = expanded;
    const numbers = gutter.current!.children;
    Array.from(mirror.current!.children).forEach((line, index) => {
      (numbers[index] as HTMLElement).style.height =
        getComputedStyle(line).height;
    });
    syncScroll();
  }
  useLayoutEffect(() => {
    if (cursor.current !== undefined) {
      input.current!.setSelectionRange(
        cursor.current.start,
        cursor.current.end,
        cursor.current.direction,
      );
      cursor.current = undefined;
    }
    measure();
    if (followOnEdit.current) {
      input.current!.scrollTop = input.current!.scrollHeight;
      syncScroll();
      followOnEdit.current = false;
    }
  }, [value, composition, editorSettings.tabSize, editorSettings.fontFamily]);
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
    if (readOnly) return;
    // Edit with native LF line breaks; untouched chips retain their original bytes.
    text = text.replaceAll("\r\n", "\n").replaceAll("\r", "\n");
    caret ??= start + text.length;
    selectionEnd ??= caret;
    const el = input.current!;
    const next = value.slice(0, start) + text + value.slice(end);
    expectedEdit.current = next;
    // Native input can commit React synchronously, including async file drops.
    // Stage the caret before that commit so it cannot rewind the next keystroke.
    cursor.current = { start: caret, end: selectionEnd, direction };
    el.focus({ preventScroll: true });
    el.setSelectionRange(start, end);
    // Native insertion preserves the browser's undo stack and selection behavior.
    // setRangeText is the fallback for engines without insertText support.
    document.execCommand("insertText", false, text);
    if (el.value !== next) {
      el.setRangeText(text, start, end, "end");
      onChange(next);
    }
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
      kind: "paste",
      label: () => t(range.paste.body ? "Paste source" : "Attachment"),
      title: () =>
        range.paste.attachment?.name ??
        t("Paste · {lines} lines · {bytes}B", {
          lines: range.paste.lines,
          bytes: range.paste.bytes.toLocaleString(currentLocale()),
        }),
      content: (close) => (
        <PastePreview
          paste={range.paste}
          retry={() => {
            if (!latest.current.readOnly) startUpload(range.paste);
          }}
          apply={(text) => {
            // Cards are non-modal: editing behind a preview must never replace a
            // different occurrence at a stale offset.
            if (latest.current.readOnly || latest.current.value !== value)
              return false;
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
      <div
        className="composer-editor"
        data-composing={composition}
        data-decorated={decorated}
        data-command-open={commandHints.open}
        data-file-drag={dragging}
        onDragOver={(event) => {
          if (!event.dataTransfer.types.includes("Files")) return;
          event.preventDefault();
          event.dataTransfer.dropEffect = upload && !readOnly ? "copy" : "none";
          setDragging(!!upload && !readOnly);
        }}
        onDragLeave={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null))
            setDragging(false);
        }}
        onDrop={(event) => {
          if (!event.dataTransfer.types.includes("Files")) return;
          event.preventDefault();
          setDragging(false);
          if (!readOnly && upload)
            void insertPicked(droppedFiles(event.dataTransfer));
        }}
        style={{
          fontFamily: editorSettings.fontFamily,
          tabSize: editorSettings.tabSize,
          ...paletteVariables(editorSettings.colorPalette, theme),
        }}
      >
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
            {...commandHints.inputProps}
            aria-description={
              tabMovesFocus
                ? t("Tab: Move focus. Ctrl+M: Switch to indentation mode.")
                : t(
                    "Tab: {action}. Shift+Tab: Outdent. Ctrl+M: Toggle Tab focus traversal.",
                    {
                      action: editorSettings.insertSpaces
                        ? t("Indent {count} spaces", {
                            count: editorSettings.indentSize,
                          })
                        : t("Insert a tab character"),
                    },
                  )
            }
            placeholder={placeholder}
            value={value}
            readOnly={readOnly}
            rows={blocks.length ? Math.min(12, Math.max(5, lines.length)) : 3}
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
            wrap="soft"
            onFocus={commandHints.updateSelection}
            onBlur={commandHints.updateSelection}
            onKeyUp={commandHints.updateSelection}
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
              commandHints.updateSelection();
            }}
            onBeforeInput={() => {
              if (!composing.current) normalizedSelection();
            }}
            onChange={(event) => {
              if (readOnly) return;
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
              followOnEdit.current =
                event.target.selectionStart === event.target.selectionEnd &&
                next.indexOf("\n", event.target.selectionEnd) < 0;
              onChange(next);
              commandHints.updateSelection();
            }}
            onPaste={(event) => {
              if (readOnly) {
                event.preventDefault();
                return;
              }
              const text = event.clipboardData.getData("text/plain");
              if (upload && event.clipboardData.files.length) {
                event.preventDefault();
                void insertPicked(
                  pickedFiles(Array.from(event.clipboardData.files)),
                );
                return;
              }
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
                setNotice("Pastes are limited to 1 MiB.");
                return;
              }
              if (
                paste.bytes +
                  [...pastes.values()].reduce(
                    (sum, p) => sum + (p.body ? p.bytes : 0),
                    0,
                  ) >
                MAX_PASTE_CACHE_BYTES
              ) {
                setNotice("Paste storage has reached 32 MiB.");
                return;
              }
              if (upload) {
                const id = paste.token.slice(7, 15);
                const file = new File([text], `paste-${id}.txt`, {
                  type: "text/plain",
                });
                paste = {
                  ...createFileChip(file, file.name),
                  body: text,
                  lines: paste.lines,
                };
              }
              pastes.set(paste.token, paste);
              setNotice("");
              replace(selection.start, selection.end, paste.token);
              if (upload) startUpload(paste);
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
              if (readOnly) return;
              if (event.nativeEvent.isComposing || composing.current) return;
              if (commandHints.key(event)) return;
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
                    ? t(
                        "Tab: {action} · Shift+Tab: Outdent · Ctrl+M: Focus traversal mode",
                        {
                          action: editorSettings.insertSpaces
                            ? t("Indent {count} spaces", {
                                count: editorSettings.indentSize,
                              })
                            : t("Insert a tab character"),
                        },
                      )
                    : t("Tab: Move focus · Ctrl+M: Indentation mode"),
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
                  editorSettings,
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
                const inline = inlineBacktickEdit(
                  value,
                  selection.start,
                  blocks,
                );
                if (inline) {
                  event.preventDefault();
                  if (inline.skip)
                    el.setSelectionRange(inline.caret, inline.caret);
                  else
                    replace(
                      selection.start,
                      selection.end,
                      inline.text,
                      inline.caret,
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
                } else setNotice("There are no paste chips in this input.");
                return;
              }
              if (
                event.key === "Enter" &&
                !event.ctrlKey &&
                !event.metaKey &&
                !event.altKey &&
                !event.shiftKey
              ) {
                const edit = listNewlineEdit(
                  value,
                  selection.start,
                  selection.end,
                  blocks,
                );
                if (edit) {
                  event.preventDefault();
                  replace(edit.from, edit.to, edit.text, edit.caret);
                  return;
                }
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
          {commandHints.overlay}
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
                const inline = block ? [] : inlineCodeRanges(line);
                function text(from: number, to: number) {
                  if (tokens)
                    return tokenSpans(tokens, from - start, to - start);
                  if (!inline.length || from === to)
                    return value.slice(from, to);
                  let offset = from - start;
                  const end = to - start;
                  const parts = inline.flatMap((range) => {
                    const left = Math.max(offset, range.start);
                    const right = Math.min(end, range.end);
                    if (left >= right) return [];
                    const plain = line.slice(offset, left);
                    offset = right;
                    return [
                      plain,
                      <span className="editor-inline-code" key={left}>
                        {line.slice(left, right)}
                      </span>,
                    ];
                  });
                  return [...parts, line.slice(offset, end)];
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
                      data-upload-state={range.paste.attachment?.state}
                      role="button"
                      tabIndex={0}
                      aria-label={
                        range.paste.attachment
                          ? `${range.paste.attachment.name} · ${t(range.paste.attachment.state === "uploading" ? "Uploading…" : range.paste.attachment.state === "error" ? "Upload failed." : "Attachment")}`
                          : t(
                              "View paste source: {lines} lines, {bytes} bytes",
                              {
                                lines: range.paste.lines,
                                bytes: range.paste.bytes,
                              },
                            )
                      }
                      title={
                        range.paste.attachment?.error ??
                        (range.paste.attachment?.state === "uploading"
                          ? t("Uploading…")
                          : t("View source · Ctrl+P"))
                      }
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
                            disabled={readOnly}
                            aria-label={t("Code syntax {index}", {
                              index: blocks.indexOf(block!) + 1,
                            })}
                            value={codeSyntax(block!.syntax)}
                            onChange={(event) =>
                              changeSyntax(block!, event.target.value)
                            }
                          >
                            {codeSyntaxes.map((syntax) => (
                              <option key={syntax} value={syntax}>
                                {syntax === "auto"
                                  ? t("Auto · {syntax}", {
                                      syntax: block!.detected,
                                    })
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
                            disabled={readOnly}
                            type="button"
                            aria-label={t("Close code block {index}", {
                              index: blocks.indexOf(block!) + 1,
                            })}
                            title={t("Finish code block")}
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
          {translateKnown(notice)}
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
  retry,
}: {
  paste: ComposerPaste;
  apply: (text: string) => boolean;
  retry: () => void;
}) {
  useLocale();
  const [stale, setStale] = useState(false);
  const [, refresh] = useState(0);
  useEffect(() => {
    if (!paste.attachment) return;
    const listeners = (paste.attachment.listeners ??= new Set());
    const listener = () => refresh((v) => v + 1);
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  }, [paste]);
  return (
    <>
      {paste.attachment && (
        <p className="attachment-info">
          {paste.attachment.name} ·{" "}
          {paste.bytes.toLocaleString(currentLocale())} B
          {paste.attachment.state === "uploading" && (
            <span role="status"> · {t("Uploading…")}</span>
          )}
          {paste.attachment.error && (
            <span role="alert">
              {" "}
              · {translateKnown(paste.attachment.error)}
            </span>
          )}
        </p>
      )}
      {paste.attachment?.path && (
        <p className="attachment-info">
          <code>{paste.attachment.path}</code>
        </p>
      )}
      {paste.body && <pre>{paste.body}</pre>}
      <div className="buttons">
        {paste.attachment?.state === "error" && (
          <Button
            type="button"
            onClick={retry}
            disabled={!paste.attachment.file}
          >
            {t("Retry upload")}
          </Button>
        )}
        {paste.body && (
          <Button
            type="button"
            disabled={stale}
            onClick={() => setStale(!apply(paste.body))}
          >
            {t("Expand source")}
          </Button>
        )}
        <Button
          type="button"
          disabled={stale}
          onClick={() => setStale(!apply(""))}
        >
          {t("Delete")}
        </Button>
      </div>
      {stale && (
        <p role="status" className="muted">
          {t("The input has changed. Reopen the chip.")}
        </p>
      )}
    </>
  );
}
