import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { Store, key } from "@lesomnus/payday/store";
import { openDisk, deleteOtherDisks } from "@lesomnus/payday/store/idb";
import { Task as TaskEntity, Project } from "#gen/entities";
import type { Task } from "#gen/cxz/task_pb";
import { TaskListResponseSchema } from "#gen/cxz/task_svc_pb";

const entities = [TaskEntity, Project];
const listKey = (project: string) => `project/${project}`;

// Watch has an application-specific project scope, so its membership is kept
// beside Payday's normalized rows instead of using Queries' generic Watch.
export class TaskCache {
  constructor(readonly store: Store) {}

  read(project: string): Task[] | undefined {
    const bytes = this.store.blob(listKey(project));
    if (!bytes) return;
    try {
      return fromBinary(TaskListResponseSchema, bytes).items.flatMap((item) => {
        const value = this.store.get<Task>(TaskEntity.typeName, item.id);
        return value ? [value] : [];
      });
    } catch {
      this.store.setBlob(listKey(project), undefined);
    }
  }

  write(project: string, tasks: Task[]) {
    const previous = this.read(project) ?? [];
    const ids = new Set(tasks.map((task) => key(task.id)));
    this.store.apply(TaskEntity.typeName, [
      ...previous
        .filter((task) => !ids.has(key(task.id)))
        .map(({ id }) => ({ id })),
      ...tasks.map((value) => ({ id: value.id, value })),
    ]);
    this.store.setBlob(
      listKey(project),
      toBinary(
        TaskListResponseSchema,
        create(TaskListResponseSchema, {
          items: tasks.map(({ id }) => ({ id })),
        }),
      ),
    );
  }

  forget() {
    return this.store.forget();
  }
  close() {
    return this.store.close();
  }
}

export async function openTaskCache(baseUrl: string, identity: string) {
  const at = { name: `cxz.tasks:${new URL(baseUrl).origin}`, identity };
  const disk = await openDisk(entities, at);
  const store = Store.open(entities, { ...at, disk });
  try {
    await store.hydrate();
  } catch (error) {
    await store.close().catch(() => {});
    throw error;
  }
  void deleteOtherDisks(at);
  return new TaskCache(store);
}
