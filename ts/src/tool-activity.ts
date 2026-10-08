import type { SessionEvent, ToolFileSummary } from "../gen/cxz/session_pb";
import { isTranscriptEvent, payload } from "./journal";
import { responseCompletionIndex } from "./response-completion";

export type ToolActivity = {
  call?: SessionEvent;
  result?: SessionEvent;
  output: SessionEvent[];
  approvals: { request: SessionEvent; resolution?: SessionEvent }[];
};
export type ToolFile = Omit<ToolFileSummary, "$typeName" | "$unknown">;

const textLines = (text: string) =>
  text === "" ? 0 : text.replace(/\n$/, "").split("\n").length;

function nativeFiles(event: SessionEvent): ToolFile[] {
  const p = payload(event);
  if (p.item?.type === "fileChange" && Array.isArray(p.item.changes))
    return p.item.changes
      .filter((v: unknown) => v && typeof v === "object")
      .map((change: any) => {
        const kind = change.kind;
        let added = 0,
          removed = 0,
          inHunk = false,
          known = false;
        for (const line of typeof change.diff === "string"
          ? change.diff.split("\n")
          : []) {
          if (line.startsWith("@@ ")) known = inHunk = true;
          else if (line.startsWith("diff --git ")) inHunk = false;
          else if (inHunk && line.startsWith("+")) added++;
          else if (inHunk && line.startsWith("-")) removed++;
        }
        return {
          path: typeof change.path === "string" ? change.path : "",
          movePath: typeof kind?.move_path === "string" ? kind.move_path : "",
          action:
            typeof kind === "string"
              ? kind
              : typeof kind?.type === "string"
                ? kind.type
                : "edit",
          added,
          removed,
          lines: 0,
          measure: known ? "diff" : "unknown",
          perMatch: false,
        };
      });
  const input = p.input && typeof p.input === "object" ? p.input : p;
  if (event.kind !== "tool_call") return [];
  if (!["Read", "Write", "Edit"].includes(event.text)) return [];
  const file: ToolFile = {
    path: typeof input.file_path === "string" ? input.file_path : "",
    movePath: "",
    action: event.text.toLowerCase(),
    added: 0,
    removed: 0,
    lines: 0,
    measure: event.text === "Read" ? "" : "unknown",
    perMatch: false,
  };
  if (event.text === "Write" && typeof input.content === "string") {
    file.measure = "content";
    file.lines = textLines(input.content);
  } else if (
    event.text === "Edit" &&
    typeof input.old_string === "string" &&
    typeof input.new_string === "string"
  ) {
    file.measure = "replacement";
    file.added = textLines(input.new_string);
    file.removed = textLines(input.old_string);
    file.perMatch = input.replace_all === true;
  }
  return [file];
}

// Completion can supply Codex paths/diffs that were absent on its start event.
// A live result also enriches a historical summary without fetching details.
export function toolFiles(activity: ToolActivity) {
  for (const event of [activity.result, activity.call, ...activity.output]) {
    if (!event) continue;
    const files = event.toolSummary?.files.length
      ? event.toolSummary.files
      : nativeFiles(event);
    if (files.length)
      return { files, omitted: event.toolSummary?.omittedFiles ?? 0 };
  }
  return { files: [] as ToolFile[], omitted: 0 };
}

export function fileAction(file: ToolFile) {
  return (
    (
      {
        write: "Write",
        add: "Write",
        edit: "Edit",
        update: "Edit",
        delete: "Delete",
        read: "Read",
      } as Record<string, string>
    )[file.action] ||
    file.action ||
    "Edit"
  );
}
export function fileMeasure(file: ToolFile) {
  const count =
    file.measure === "content"
      ? `+${file.lines} content`
      : ["replacement", "diff"].includes(file.measure)
        ? `+${file.added} -${file.removed}`
        : file.measure === "unknown"
          ? "Δ?"
          : "";
  return count + (file.perMatch ? " /match" : "");
}
const key = (e: SessionEvent, id = e.requestId) =>
  JSON.stringify([e.runId, id]);
const toolKinds = new Set(["tool_call", "tool_output", "tool_result"]);

// Group by native execution identity, never by command text: concurrent tools
// can run the same command, and a new run can reuse an old request ID.
export function transcriptEvents(events: SessionEvent[]) {
  const { completions, representedEnds } = responseCompletionIndex(events);
  const groups = new Map<string, ToolActivity>();
  const resolutions = new Map<string, SessionEvent>();
  for (const e of events) {
    if (e.kind === "approval_resolved") resolutions.set(key(e), e);
    if (!toolKinds.has(e.kind) || !e.requestId) continue;
    const group: ToolActivity = groups.get(key(e)) ?? {
      output: [],
      approvals: [],
    };
    if (e.kind === "tool_call") group.call = e;
    else if (e.kind === "tool_result") group.result = e;
    else group.output.push(e);
    groups.set(key(e), group);
  }
  const pairedApprovals = new Set<SessionEvent>();
  for (const e of events) {
    if (e.kind !== "approval") continue;
    if (["AskUserQuestion", "item/tool/requestUserInput"].includes(e.text))
      continue;
    const p = payload(e);
    const id = p.tool_use_id || p.params?.itemId;
    const group =
      typeof id === "string" && id ? groups.get(key(e, id)) : undefined;
    if (!group) continue;
    const resolution = resolutions.get(key(e));
    group.approvals.push({
      request: e,
      resolution: resolution && resolution.seq > e.seq ? resolution : undefined,
    });
    pairedApprovals.add(e);
  }
  const activities = new Map<bigint, ToolActivity>();
  const rows = events.filter((e) => {
    if (
      !isTranscriptEvent(e) ||
      pairedApprovals.has(e) ||
      representedEnds.has(e.seq)
    )
      return false;
    const group =
      toolKinds.has(e.kind) && e.requestId ? groups.get(key(e)) : undefined;
    if (!group) {
      if (toolKinds.has(e.kind))
        activities.set(e.seq, {
          call: e.kind === "tool_call" ? e : undefined,
          result: e.kind === "tool_result" ? e : undefined,
          output: e.kind === "tool_output" ? [e] : [],
          approvals: [],
        });
      return true;
    }
    // Retain orphan results/output at a loaded-history edge rather than dropping
    // them. Once the call is loaded, its original row owns the whole activity.
    const anchor = group.call ?? group.output[0] ?? group.result;
    if (e !== anchor) return false;
    activities.set(e.seq, group);
    return true;
  });
  return { events: rows, activities, completions };
}

export function toolState(activity: ToolActivity, agent: string) {
  const summarized =
    activity.result?.toolSummary ??
    activity.call?.toolSummary ??
    activity.output[0]?.toolSummary;
  if (
    activity.result?.toolSummary &&
    ["pending", "working", "completed", "failed"].includes(
      activity.result.toolSummary.state,
    )
  )
    return activity.result.toolSummary.state as
      | "pending"
      | "working"
      | "completed"
      | "failed";
  if (activity.result) {
    const p = payload(activity.result);
    const state = p.item?.status;
    return p.is_error === true ||
      (typeof p.item?.exitCode === "number" && p.item.exitCode !== 0) ||
      [
        "failed",
        "denied",
        "declined",
        "canceled",
        "cancelled",
        "interrupted",
      ].includes(state)
      ? "failed"
      : "completed";
  }
  const decision = activity.approvals.at(-1);
  if (decision) {
    if (!decision.resolution) return "pending";
    return decision.resolution.text === "allowed" ? "working" : "failed";
  }
  if (summarized && ["completed", "failed"].includes(summarized.state))
    return summarized.state as "completed" | "failed";
  if (
    activity.output.length ||
    (agent === "codex" &&
      activity.call &&
      payload(activity.call).item?.status === "inProgress")
  )
    return "working";
  return summarized?.state === "working" ? "working" : "pending";
}

// Parse shell quoting for display only. Do not evaluate variables, substitutions
// or scripts. Unrecognized/ambiguous wrappers retain the full native command.
function shellWords(text: string): string[] | undefined {
  const words: string[] = [];
  let word = "",
    quote = "",
    started = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (c === "\\" && quote !== "'") {
      const next = text[++i];
      if (next === undefined) return;
      if (next !== "\n")
        word += quote === '"' && !'$`"\\'.includes(next) ? "\\" + next : next;
      started = true;
    } else if (quote) {
      if (c === quote) quote = "";
      else word += c;
    } else if (c === "'" || c === '"') {
      quote = c;
      started = true;
    } else if (/\s/.test(c)) {
      if (started) words.push(word);
      word = "";
      started = false;
    } else {
      word += c;
      started = true;
    }
  }
  if (quote) return;
  if (started) words.push(word);
  return words;
}

export function shellCommand(command: string) {
  const original = { shell: "", command };
  const words = shellWords(command);
  if (!words || words.length < 3) return original;
  const shell = words[0].split("/").at(-1)!;
  if (!["sh", "bash", "zsh", "dash", "ksh", "ash"].includes(shell))
    return original;
  let carries = false;
  for (let i = 1; i < words.length - 1; i++) {
    const option = words[i];
    if (option === "-o" || option === "+o") i++;
    else if (option.startsWith("--")) continue;
    else if (option.startsWith("-")) carries ||= option.slice(1).includes("c");
    else return original;
  }
  const script = words.at(-1)!;
  return carries && script.trim() ? { shell, command: script } : original;
}

export function toolLabel(activity: ToolActivity, agent: string) {
  const event = activity.call ?? activity.result ?? activity.output[0];
  if (!event) return { name: "Tool output", shell: "", command: "" };
  if (event.toolSummary) return event.toolSummary;
  const p = payload(event);
  if (
    (agent === "codex" && p.item?.type === "commandExecution") ||
    (agent === "claude" && event.text === "Bash")
  ) {
    const command = agent === "codex" ? p.item.command : p.command;
    return {
      name: "Bash",
      ...shellCommand(
        typeof command === "string" ? command : (activity.call?.text ?? ""),
      ),
    };
  }
  if (agent === "codex" && p.item?.type === "fileChange") {
    return {
      name: "Files",
      shell: "",
      command: (Array.isArray(p.item.changes) ? p.item.changes : [])
        .map((v: { path?: string }) => v.path)
        .filter(Boolean)
        .join(", "),
    };
  }
  return {
    name: activity.call?.text || "Tool result",
    shell: "",
    command: typeof p.file_path === "string" ? p.file_path : "",
  };
}

export function toolOutput(activity: ToolActivity) {
  if (!activity.result) return activity.output.map((e) => e.text).join("");
  const p = payload(activity.result);
  if (typeof p.item?.aggregatedOutput === "string")
    return p.item.aggregatedOutput;
  if (activity.result.text) return activity.result.text;
  if (typeof p.content === "string") return p.content;
  if (Array.isArray(p.content))
    return p.content.map((v: { text?: string }) => v.text ?? "").join("\n");
  return "";
}
