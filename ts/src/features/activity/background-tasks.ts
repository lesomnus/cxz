export type BackgroundTask = {
  task_id: string;
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
  return backgroundStateTasks(states[run]);
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
