export type BackgroundTask = {
  task_id: string;
  tool_use_id?: string;
  updated_seq?: string;
  updated_at_ms?: number;
  description: string;
  status: string;
  summary: string;
  output_file: string;
  active: boolean;
};
export function backgroundTasks(
  bytes: Uint8Array,
  run: string,
): BackgroundTask[] {
  const states = JSON.parse(new TextDecoder().decode(bytes) || "{}");
  return visibleBackgroundTasks(backgroundStateTasks(states[run]));
}
export function backgroundStateTasks(
  state: { Tasks?: unknown } | undefined,
): BackgroundTask[] {
  const tasks = state?.Tasks;
  if (!tasks || typeof tasks !== "object") return [];
  return Object.values(tasks).filter(
    (task): task is BackgroundTask =>
      !!task &&
      typeof task === "object" &&
      typeof (task as BackgroundTask).task_id === "string",
  );
}

const finishedLimit = 3;
const sequence = (task: BackgroundTask) =>
  typeof task.updated_seq === "string" && /^\d+$/.test(task.updated_seq)
    ? BigInt(task.updated_seq)
    : 0n;

/** The display limit does not remove records from the provider snapshot. */
export function visibleBackgroundTasks(tasks: readonly BackgroundTask[]) {
  const recent = (a: BackgroundTask, b: BackgroundTask) => {
    const delta = sequence(b) - sequence(a);
    return delta > 0n
      ? 1
      : delta < 0n
        ? -1
        : (b.updated_at_ms ?? 0) - (a.updated_at_ms ?? 0) ||
          a.task_id.localeCompare(b.task_id);
  };
  return [
    ...tasks.filter((task) => task.active === true).sort(recent),
    ...tasks
      .filter((task) => task.active !== true)
      .sort(recent)
      .slice(0, finishedLimit),
  ];
}
