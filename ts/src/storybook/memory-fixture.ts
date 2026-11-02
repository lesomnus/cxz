import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createRouterTransport } from "@connectrpc/connect";
import { ProjectSchema } from "#gen/cxz/project_pb";
import { SessionSchema } from "#gen/cxz/session_pb";
import {
  LibraryDocuments,
  LibraryMemorySchema,
  LibraryScope,
  SessionService,
  type LibraryMemory,
} from "#gen/cxz/session_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { projectFixtureId } from "./project-id";

type Document = { content: string; revision: string; deleted?: boolean };
type Entry = { memory: LibraryMemory; documents: Map<string, Document> };

/** Project-scoped data, including an empty editing session and populated peers. */
export function memoryFixture({
  empty = false,
  fail = false,
  delay = 0,
  conflict = false,
} = {}) {
  const projects = ["web", "api"].map((alias, index) =>
    create(ProjectSchema, {
      id: projectFixtureId(index + 1),
      alias,
      name: index ? "API workspace" : "Web workspace",
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
  ].map((s, index) =>
    create(SessionSchema, { ...s, id: projectFixtureId(index + 100) }),
  );
  function entry(
    id: string,
    name: string,
    session: string,
    content?: string,
    snapshot = false,
  ): Entry {
    return {
      memory: create(LibraryMemorySchema, {
        id,
        name,
        session,
        agent: session === "writer" ? "claude" : "codex",
        snapshot,
        updated: timestampFromDate(new Date("2026-11-01T18:00:00Z")),
      }),
      documents: new Map(
        content === undefined ? [] : [["notes.md", { content, revision: "1" }]],
      ),
    };
  }
  const libraries = [
    new Map<string, Entry>(
      empty
        ? []
        : [
            ["own", entry("own", "New session", "demo")],
            [
              "shared",
              entry(
                "shared",
                "Architecture notes",
                "writer",
                "# Architecture\n\nShared memory belongs to the project, even when this session is new.\n\n- Keep UI decisions together.\n- Preserve document revisions.",
              ),
            ],
            [
              "checkpoint",
              entry(
                "checkpoint",
                "Saved checkpoint",
                "writer",
                "# Saved checkpoint\n\nRead only snapshot.",
                true,
              ),
            ],
          ],
    ),
    new Map([
      [
        "api-own",
        entry(
          "api-own",
          "API notes",
          "api-demo",
          "# API\n\nAPI-specific shared memory.",
        ),
      ],
    ]),
  ];
  let serial = 0;
  const transport = createRouterTransport(({ service }) =>
    service(SessionService, {
      libraryMemories: async (request) => {
        if (delay) await new Promise((resolve) => setTimeout(resolve, delay));
        if (fail)
          throw new ConnectError(
            "Memory library temporarily unavailable",
            Code.Unavailable,
          );
        return {
          memories: [...library(request).values()]
            .filter(
              (e) =>
                !!e.memory.deleted === (request.scope === LibraryScope.TRASH) &&
                (!request.query ||
                  `${e.memory.name} ${[...e.documents.values()].map((d) => d.content).join(" ")}`
                    .toLowerCase()
                    .includes(request.query.toLowerCase())),
            )
            .map((e) => metadata(e)),
        };
      },
      libraryMemory: (request) => {
        const e = get(request);
        return {
          memory: metadata(e),
          documents: [...e.documents]
            .filter(
              ([, d]) =>
                !!d.deleted ===
                (request.documents === LibraryDocuments.DELETED),
            )
            .map(([name, d]) => ({
              name,
              revision: d.revision,
              size: BigInt(d.content.length),
            })),
        };
      },
      libraryDocument: (request) => {
        const e = get(request);
        const d = e.documents.get(request.document);
        if (!d || d.deleted)
          throw new ConnectError("Document not found", Code.NotFound);
        return { memory: metadata(e), ...d };
      },
      writeLibraryDocument: (request) => {
        if (conflict)
          throw new ConnectError(
            "Document revision changed; reload before saving",
            Code.Aborted,
          );
        const e = own(request);
        const previous = e.documents.get(request.document);
        if (previous && previous.revision !== request.revision)
          throw new ConnectError("Document revision changed", Code.Aborted);
        const d = {
          content: request.content,
          revision: String(Number(previous?.revision ?? "0") + 1),
        };
        e.documents.set(request.document, d);
        return { memory: metadata(e), ...d };
      },
      renameLibraryMemory: (request) => {
        const e = get(request);
        e.memory.name = request.name;
        return { memory: metadata(e) };
      },
      snapshotLibraryMemory: (request) => {
        const source = get(request);
        const id = `snapshot-${++serial}`;
        const saved = entry(
          id,
          request.name || "New checkpoint",
          source.memory.session,
          undefined,
          true,
        );
        saved.documents = new Map(
          [...source.documents].map(([name, d]) => [name, { ...d }]),
        );
        library(request).set(id, saved);
        return { memory: metadata(saved) };
      },
      forkLibraryMemory: (request) => {
        const source = get(request);
        const target = own(request);
        target.documents = new Map(
          [...source.documents].map(([name, d]) => [name, { ...d }]),
        );
        target.memory.name = request.name || "Forked notes";
        return { memory: metadata(target) };
      },
      mergeLibraryMemories: (request) => {
        const source = request.ids.map((id) => get({ ...request, id }));
        const target = own(request);
        for (const e of source)
          for (const [name, d] of e.documents)
            target.documents.set(name, { ...d });
        target.memory.name = request.name || "Merged notes";
        return { memory: metadata(target) };
      },
      deleteLibraryMemory: (request) => {
        get(request).memory.deleted = true;
        return {};
      },
      restoreLibraryMemory: (request) => {
        get(request).memory.deleted = false;
        return {};
      },
      purgeLibraryMemory: (request) => {
        library(request).delete(request.id);
        return {};
      },
      deleteLibraryDocument: (request) => {
        get(request).documents.get(request.document)!.deleted = true;
        return {};
      },
      restoreLibraryDocument: (request) => {
        get(request).documents.get(request.document)!.deleted = false;
        return {};
      },
      purgeLibraryDocument: (request) => {
        get(request).documents.delete(request.document);
        return {};
      },
      importLibraryMemories: () => ({}),
    }),
  );
  type Context = { ref?: { key: { case?: string; value?: unknown } } };
  function runtime(request: Context) {
    return String(request.ref?.key.value);
  }
  function library(request: Context) {
    return libraries[runtime(request) === "api-demo" ? 1 : 0]!;
  }
  function get(request: Context & { id: string }) {
    const e = library(request).get(request.id);
    if (!e)
      throw new ConnectError(
        "Memory not found (explicit memory ID required)",
        Code.NotFound,
      );
    return e;
  }
  function own(request: Context) {
    const items = library(request);
    const session = runtime(request);
    let e = [...items.values()].find(
      (v) =>
        v.memory.session === session && !v.memory.snapshot && !v.memory.deleted,
    );
    if (!e) {
      const id = `own-${session}`;
      e = entry(id, "New session", session);
      items.set(id, e);
    }
    return e;
  }
  function metadata(e: Entry) {
    return {
      ...e.memory,
      size: BigInt(
        [...e.documents.values()]
          .filter((d) => !d.deleted)
          .reduce((n, d) => n + d.content.length, 0),
      ),
    };
  }
  return {
    c: new Connection("http://storybook", transport),
    projects,
    sessions,
  };
}
