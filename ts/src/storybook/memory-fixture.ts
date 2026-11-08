import { create, isFieldSet } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { Id } from "@lesomnus/payday/pdid";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import { MemorySchema, type Memory } from "#gen/cxz/memory_pb";
import {
  MemoryService,
  MemoryPatchRequestSchema,
  MemoryScope,
  MemoryReadFormat,
  MemoryVersionSchema,
  type MemoryVersion,
} from "#gen/cxz/memory_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { projectFixtureId } from "./project-id";
import {
  memoryParagraphBefore,
  memoryParagraphAfter,
} from "./memory-long-paragraph";
export function memoryFixture({
  empty = false,
  fail = false,
  delay = 0,
  conflict = false,
  migrated = false,
  cards = false,
  readDelay = 0,
  readLoading = false,
  revisionCount = 1,
  missingPreviousRevision = false,
  diffFailOnce = false,
  longParagraphChange = false,
} = {}) {
  const projects = ["web", "api"].map((alias, i) =>
    create(ProjectSchema, {
      id: projectFixtureId(i + 1),
      alias,
      name: i ? "API workspace" : "Web workspace",
    }),
  );
  const sessions = [
    {
      runtimeId: "demo",
      alias: "oak",
      name: "New editing session",
      agent: "codex",
      project: projects[0],
    },
    {
      runtimeId: "writer",
      alias: "elm",
      name: "Design decisions",
      agent: "claude",
      project: projects[0],
    },
    {
      runtimeId: "api-demo",
      alias: "pine",
      name: "API implementation",
      agent: "codex",
      project: projects[1],
    },
  ].map((s, i) =>
    create(SessionSchema, { ...s, id: projectFixtureId(i + 100) }),
  );
  const rows = new Map<string, Memory>(),
    versions = new Map<string, Map<bigint, string>>(),
    versionInfo = new Map<string, Map<bigint, MemoryVersion>>();
  let serial = 200,
    tick = Date.now();
  const stamp = () => timestampFromDate(new Date(++tick));
  const key = (id: Uint8Array) => new Id(id).toString();
  function add(
    name: string,
    content: string,
    p = 0,
    owner = sessions[p === 0 ? 1 : 2],
  ) {
    const id = projectFixtureId(++serial);
    id[9] = 18;
    const m = create(MemorySchema, {
      id,
      name,
      project: projects[p],
      ownerSession: key(owner!.id),
      revision: 1n,
      content,
      dateCreated: stamp(),
      dateUpdated: stamp(),
      labels: { topic: p ? "api" : "design" },
    });
    rows.set(key(id), m);
    versions.set(key(id), new Map([[1n, content]]));
    versionInfo.set(
      key(id),
      new Map([
        [
          1n,
          create(MemoryVersionSchema, {
            memoryId: id,
            revision: 1n,
            contentHash: "hash-1",
            dateCreated: m.dateCreated,
          }),
        ],
      ]),
    );
    return m;
  }
  if (!empty) {
    const primary = add(
      "Architecture notes",
      longParagraphChange
        ? memoryParagraphBefore
        : "# Architecture\n\nShared memory belongs to the project.\n\nKeep UI decisions together.",
    );
    add("Handoff notes", "# Handoff\n\nReview open changes.");
    if (revisionCount > 1) {
      primary.dateCreated = timestampFromDate(
        new Date(Date.now() - (revisionCount - 1) * 86400000),
      );
      versionInfo.get(key(primary.id))!.get(1n)!.dateCreated =
        primary.dateCreated;
    }
    for (let revision = 2; revision <= revisionCount; revision++) {
      primary.revision = BigInt(revision);
      primary.content = longParagraphChange
        ? memoryParagraphAfter
        : `# Architecture\n\nShared memory belongs to the project.\n\nDesign decisions from revision ${revision}.`;
      versions.get(key(primary.id))!.set(primary.revision, primary.content);
      versionInfo.get(key(primary.id))!.set(
        primary.revision,
        create(MemoryVersionSchema, {
          memoryId: primary.id,
          revision: primary.revision,
          contentHash: `hash-${revision}`,
          dateCreated: timestampFromDate(
            new Date(Date.now() - (revisionCount - revision) * 86400000),
          ),
          linesAdded: 1,
          linesRemoved: 1,
        }),
      );
    }
    primary.dateUpdated = stamp();
    if (missingPreviousRevision && primary.revision > 1n)
      versions.get(key(primary.id))!.delete(primary.revision - 1n);
  }
  if (migrated && !empty) {
    for (let i = 0; i < 24; i++) {
      const filename = [
        "decisions.md",
        "overview.md",
        "web-ui.md",
        "runtime.md",
      ][i % 4]!;
      const m = add(
        filename,
        `# Project notes ${i + 1}\n\nA migrated document is still one editable Memory.\n\n## Design decisions\n\n- Keep project knowledge available after its session stops.\n- Organize documents with labels.\n\n\`\`\`typescript\nconst memory = { project: 'web', revision: 1 };\n\`\`\``,
        0,
        sessions[i % 2],
      );
      m.labels = {
        "legacy.collection": `session-${String(i).padStart(24, "0")}`,
        "legacy.collection-name": `Session ${String(i).repeat(24)}`,
        "legacy.filename": filename,
        "legacy.agent": i % 2 ? "claude" : "codex",
        ...(i % 3 === 0 ? { topic: "design" } : {}),
      };
      if (i % 4 === 0) m.ownerSession = key(projectFixtureId(500 + i));
    }
  }
  if (cards && !empty) {
    const names = [
      "Choosing a storage model for project memories",
      "A long memory title that wraps across several lines without changing the card height",
      "Release checklist",
      "Notes without a session owner",
      "A retained session's architectural decisions",
      "Web accessibility review",
    ];
    for (const [i, name] of names.entries()) {
      const m = add(name, `# ${name}`, 0, sessions[i % 2]);
      if (i === 3) m.ownerSession = "";
      if (i === 4) {
        m.ownerSession = key(projectFixtureId(700));
        m.labels["legacy.agent"] = "claude";
      }
      m.revision = BigInt(i + 2);
      versions.get(key(m.id))!.set(m.revision, m.content);
      m.dateUpdated = timestampFromDate(
        new Date(Date.now() - (i + 1) * 3600000),
      );
    }
  }
  add("API notes", "# API\n\nAPI-specific shared memory.", 1);
  function find(ref?: {
    key: { case?: string; value?: unknown };
    project?: { key: { value?: unknown } };
  }) {
    const id = ref?.key.value;
    if (!(id instanceof Uint8Array))
      throw new ConnectError("Memory ID required", Code.InvalidArgument);
    const m = rows.get(key(id));
    const project = ref?.project?.key.value;
    if (
      !m ||
      (project instanceof Uint8Array && key(m.project!.id) !== key(project))
    )
      throw new ConnectError("Memory not found", Code.NotFound);
    return m;
  }
  const readRequests: string[] = [];
  let diffReads = 0;
  const readLifetime = new AbortController();
  const transport = createRouterTransport(({ service }) =>
    service(MemoryService, {
      list: async (r) => {
        if (delay) await new Promise((resolve) => setTimeout(resolve, delay));
        if (fail)
          throw new ConnectError(
            "Memory library temporarily unavailable",
            Code.Unavailable,
          );
        const p = r.project?.key.value;
        return {
          items: [...rows.values()]
            .filter(
              (m) =>
                (!(p instanceof Uint8Array) || key(m.project!.id) === key(p)) &&
                !!m.dateDeleted === (r.scope === MemoryScope.TRASH) &&
                (!r.session?.key.value ||
                  m.ownerSession === key(r.session.key.value as Uint8Array)) &&
                (!r.query ||
                  `${m.name} ${m.content}`
                    .toLowerCase()
                    .includes(r.query.toLowerCase())) &&
                (!r.filters.length ||
                  r.filters.some((f) =>
                    Object.entries(f.labels).every(
                      ([k, v]) => m.labels[k] === v,
                    ),
                  )),
            )
            .map((m) => ({ ...m, content: "" })),
        };
      },
      get: (r) => ({ ...find(r.ref) }),
      read: async (r, context) => {
        readRequests.push(key(r.ref!.key.value as Uint8Array));
        if (readLoading) {
          const signal = AbortSignal.any([context.signal, readLifetime.signal]);
          await new Promise<never>((_, reject) => {
            const abort = () =>
              reject(new ConnectError("Memory read canceled", Code.Canceled));
            if (signal.aborted) abort();
            else signal.addEventListener("abort", abort, { once: true });
          });
        }
        if (readDelay)
          await new Promise((resolve) => setTimeout(resolve, readDelay));
        if (
          r.format === MemoryReadFormat.DIFF &&
          ++diffReads === 1 &&
          diffFailOnce
        )
          throw new ConnectError(
            "Revision changes temporarily unavailable",
            Code.Unavailable,
          );
        const m = find(r.ref),
          history = versions.get(key(m.id))!,
          rev = r.revision || m.revision,
          content = history.get(rev);
        if (content === undefined)
          throw new ConnectError("Version not retained", Code.NotFound);
        if (
          r.format === MemoryReadFormat.DIFF &&
          r.baseRevision &&
          history.has(r.baseRevision)
        ) {
          const before = history.get(r.baseRevision)!;
          const oldLines = before.match(/[^\n]*\n|[^\n]+$/g) ?? [],
            newLines = content.match(/[^\n]*\n|[^\n]+$/g) ?? [];
          const encode = (lines: string[], prefix: string) =>
            lines.flatMap((line) =>
              line.endsWith("\n")
                ? [prefix + line.slice(0, -1)]
                : [prefix + line, "\\ No newline at end of file"],
            );
          let prefix = 0,
            suffix = 0;
          while (
            prefix < oldLines.length &&
            prefix < newLines.length &&
            oldLines[prefix] === newLines[prefix]
          )
            prefix++;
          while (
            suffix < oldLines.length - prefix &&
            suffix < newLines.length - prefix &&
            oldLines.at(-1 - suffix) === newLines.at(-1 - suffix)
          )
            suffix++;
          return {
            memory: { ...m, content: "" },
            revision: rev,
            baseRevision: r.baseRevision,
            format: MemoryReadFormat.DIFF,
            diff:
              before === content
                ? ""
                : [
                    `--- r${r.baseRevision}`,
                    `+++ r${rev}`,
                    `@@ -${oldLines.length ? 1 : 0},${oldLines.length} +${newLines.length ? 1 : 0},${newLines.length} @@`,
                    ...encode(oldLines.slice(0, prefix), " "),
                    ...encode(
                      oldLines.slice(prefix, oldLines.length - suffix),
                      "-",
                    ),
                    ...encode(
                      newLines.slice(prefix, newLines.length - suffix),
                      "+",
                    ),
                    ...encode(oldLines.slice(oldLines.length - suffix), " "),
                  ].join("\n") + "\n",
          };
        }
        return {
          memory: { ...m, content: "" },
          revision: rev,
          format: MemoryReadFormat.FULL,
          content,
          fallbackReason:
            r.format === MemoryReadFormat.DIFF && r.baseRevision
              ? "base revision is not retained"
              : "",
        };
      },
      add: (r) => {
        const retry = [...rows.values()].find(
          (m) => r.clientId && m.clientId === r.clientId,
        );
        if (retry) return retry;
        const p = r.project?.key.value;
        const index =
          p instanceof Uint8Array
            ? projects.findIndex((v) => key(v.id) === key(p))
            : 0;
        const m = add(r.name, r.content, Math.max(0, index));
        m.ownerSession = r.ownerSession;
        m.labels = { ...r.labels };
        m.clientId = r.clientId;
        return { ...m };
      },
      patch: (r) => {
        const m = find(r.ref);
        if (
          conflict ||
          (isFieldSet(r, MemoryPatchRequestSchema.field.content) &&
            r.baseRevision !== m.revision)
        )
          throw new ConnectError(
            "Memory revision changed; read current state",
            Code.Aborted,
          );
        if (
          r.dateUpdated?.seconds !== m.dateUpdated?.seconds ||
          r.dateUpdated?.nanos !== m.dateUpdated?.nanos
        )
          throw new ConnectError("Memory version changed", Code.Aborted);
        if (
          isFieldSet(r, MemoryPatchRequestSchema.field.content) &&
          r.content !== m.content
        ) {
          m.content = r.content;
          m.revision++;
          versions.get(key(m.id))!.set(m.revision, m.content);
          versionInfo.get(key(m.id))!.set(
            m.revision,
            create(MemoryVersionSchema, {
              memoryId: m.id,
              revision: m.revision,
              contentHash: `hash-${m.revision}`,
              dateCreated: stamp(),
            }),
          );
        }
        if (isFieldSet(r, MemoryPatchRequestSchema.field.name)) m.name = r.name;
        if (Object.keys(r.labels).length) m.labels = { ...r.labels };
        if (r.clearLabels) m.labels = {};
        m.dateUpdated = stamp();
        return { ...m };
      },
      versions: (r) => {
        const m = find(r.ref);
        const history = [...versions.get(key(m.id))!]
          .reverse()
          .filter(
            ([revision]) => !r.beforeRevision || revision < r.beforeRevision,
          );
        const page = history.slice(0, r.size || 100);
        return {
          nextRevision: history.length > page.length ? page.at(-1)![0] : 0n,
          versions: page.map(
            ([revision]) =>
              versionInfo.get(key(m.id))!.get(revision) ?? {
                memoryId: m.id,
                revision,
                contentHash: `hash-${revision}`,
                dateCreated: m.dateUpdated,
              },
          ),
        };
      },
      erase: (r) => {
        const m = find(r);
        m.dateDeleted = timestampFromDate(new Date(Date.now() + 30 * 86400000));
        m.dateUpdated = stamp();
        return { erased: true };
      },
      restore: (r) => {
        const m = find(r.ref);
        m.dateDeleted = undefined;
        m.dateUpdated = stamp();
        return { ...m };
      },
      purge: (r) => {
        const m = find(r.ref);
        rows.delete(key(m.id));
        versions.delete(key(m.id));
        versionInfo.delete(key(m.id));
        return { erased: true };
      },
    }),
  );
  const c = new Connection("http://storybook", transport);
  return {
    c,
    dispose() {
      readLifetime.abort();
      c.dispose();
    },
    readRequests,
    memories: [...rows.values()],
    projects,
    sessions,
  };
}
