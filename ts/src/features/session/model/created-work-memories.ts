import type { Memory } from "#gen/cxz/memory_pb";
import type { WorkGroup } from "./work-groups";

type MemoryRange = Pick<WorkGroup, "id" | "fromMs" | "toMs">;

/** Input is already scoped to its owning session by Memory.List/Watch. */
export function createdMemoriesByWorkGroup(
  memories: readonly Memory[],
  groups: Iterable<MemoryRange>,
) {
  const ranges = [...groups]
    .filter((group) => group.fromMs !== undefined)
    .sort((a, b) =>
      a.fromMs! < b.fromMs! ? -1 : a.fromMs! > b.fromMs! ? 1 : 0,
    );
  const result = new Map<string, Memory[]>();
  const ordered = memories
    .filter((memory) => memory.dateCreated && !memory.dateDeleted)
    .sort(
      (a, b) =>
        Number(a.dateCreated!.seconds - b.dateCreated!.seconds) ||
        a.dateCreated!.nanos - b.dateCreated!.nanos ||
        a.alias.localeCompare(b.alias),
    );
  for (const memory of ordered) {
    const created =
      memory.dateCreated!.seconds * 1_000_000_000n +
      BigInt(memory.dateCreated!.nanos);
    let low = 0,
      high = ranges.length;
    while (low < high) {
      const middle = (low + high) >>> 1;
      if (ranges[middle].fromMs! * 1_000_000n <= created) low = middle + 1;
      else high = middle;
    }
    const group = ranges[low - 1];
    if (
      !group ||
      (group.toMs !== undefined && created >= group.toMs * 1_000_000n)
    )
      continue;
    let matches = result.get(group.id);
    if (!matches) result.set(group.id, (matches = []));
    matches.push(memory);
  }
  return result;
}
