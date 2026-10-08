import type { SessionEvent } from "../gen/cxz/session_pb";
import { isTranscriptEvent, payload } from "./journal";
import { responseCompletionIndex } from "./response-completion";

export type ToolActivity = {
  call?: SessionEvent;
  result?: SessionEvent;
  output: SessionEvent[];
  approvals: { request: SessionEvent; resolution?: SessionEvent }[];
};
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
    if (!group) return true;
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
  if (
    activity.output.length ||
    (agent === "codex" &&
      activity.call &&
      payload(activity.call).item?.status === "inProgress")
  )
    return "working";
  return "pending";
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
  const event = activity.call ?? activity.result;
  if (!event) return { name: "Tool output", shell: "", command: "" };
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
