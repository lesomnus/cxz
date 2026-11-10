import { describe, expect, it } from "vitest";
import type { ProjectPathEntry } from "#gen/cxz/project_svc_pb";
import {
  cacheDirectory,
  directoryCacheLimit,
  toggleExpanded,
  visibleDirectories,
} from "./directory-cache";

const entry = (name: string, directory = false) =>
  ({ name, directory }) as ProjectPathEntry;
const listing = (...names: string[]) => names.map((name) => entry(name));

describe("the project's directory listings", () => {
  it("replaces a directory's entries in place rather than accumulating them", () => {
    let cache = cacheDirectory(new Map(), "/w", listing("a", "b"), {
      expanded: new Set(["/w"]),
      root: "/w",
    });
    cache = cacheDirectory(cache, "/w", listing("a"), {
      expanded: new Set(["/w"]),
      root: "/w",
    });
    expect(cache.size).toBe(1);
    expect(cache.get("/w")!.map((e) => e.name)).toEqual(["a"]);
  });

  it("drops the listings nobody is looking at once it is full", () => {
    const expanded = new Set(["/w"]);
    let cache = new Map<string, ProjectPathEntry[]>();
    for (let i = 0; i < directoryCacheLimit + 10; i++)
      cache = cacheDirectory(cache, `/w/d${i}`, listing("f"), {
        expanded,
        root: "/w",
      });
    expect(cache.size).toBe(directoryCacheLimit);
    // Oldest first, so the newest survive.
    expect(cache.has("/w/d0")).toBe(false);
    expect(cache.has(`/w/d${directoryCacheLimit + 9}`)).toBe(true);
  });

  // The tree renders from this map and asks again only when a directory is
  // expanded. Dropping an expanded one would leave it looking empty with
  // nothing to correct it.
  it("keeps what is on screen, and the root, however old", () => {
    const expanded = new Set(["/w", "/w/kept"]);
    let cache = cacheDirectory(new Map(), "/w", listing("kept"), {
      expanded,
      root: "/w",
    });
    cache = cacheDirectory(cache, "/w/kept", listing("f"), {
      expanded,
      root: "/w",
    });
    for (let i = 0; i < directoryCacheLimit + 10; i++)
      cache = cacheDirectory(cache, `/w/other${i}`, listing("f"), {
        expanded,
        root: "/w",
      });
    expect(cache.has("/w")).toBe(true);
    expect(cache.has("/w/kept")).toBe(true);
  });

  it("keeps the directory it was just given even when everything is expanded", () => {
    const expanded = new Set<string>();
    let cache = new Map<string, ProjectPathEntry[]>();
    for (let i = 0; i < 4; i++) {
      expanded.add(`/w/d${i}`);
      cache = cacheDirectory(cache, `/w/d${i}`, listing("f"), {
        expanded,
        root: "/w",
        limit: 2,
      });
    }
    expect(cache.has("/w/d3")).toBe(true);
  });
});

describe("which directories a reopened pane asks for", () => {
  it("is the root and what is open under it, not everything ever listed", () => {
    const directories = new Map<string, ProjectPathEntry[]>([
      ["/w", []],
      ["/w/src", []],
      ["/w/src/deep", []],
      ["/w/docs", []],
    ]);
    const expanded = new Set(["/w", "/w/src", "/w/src/deep"]);
    expect(visibleDirectories(directories, expanded, "/w").sort()).toEqual([
      "/w",
      "/w/src",
      "/w/src/deep",
    ]);
  });

  it("leaves out what sits under a collapsed directory", () => {
    const directories = new Map<string, ProjectPathEntry[]>([
      ["/w", []],
      ["/w/src", []],
      ["/w/src/deep", []],
    ]);
    // src is closed, so deep is not on screen however open it is.
    const expanded = new Set(["/w", "/w/src/deep"]);
    expect(visibleDirectories(directories, expanded, "/w")).toEqual(["/w"]);
  });

  it("leaves out what was evicted, so a listing is not asked for twice", () => {
    const expanded = new Set(["/w", "/w/src"]);
    expect(visibleDirectories(new Map([["/w", []]]), expanded, "/w")).toEqual([
      "/w",
    ]);
  });

  it("asks for nothing when the workspace is unknown", () => {
    expect(visibleDirectories(new Map(), new Set(["/w"]), "")).toEqual([]);
  });
  // The tree builds a child's path from a root without its trailing slash.
  it("matches a parent against a root written with a trailing slash", () => {
    const directories = new Map<string, ProjectPathEntry[]>([
      ["/w/", []],
      ["/w/src", []],
    ]);
    expect(
      visibleDirectories(directories, new Set(["/w/", "/w/src"]), "/w/"),
    ).toEqual(["/w/", "/w/src"]);
  });
});

describe("expanding and collapsing", () => {
  it("is a toggle that leaves the original set alone", () => {
    const expanded = new Set(["/w"]);
    const opened = toggleExpanded(expanded, "/w/src");
    expect([...opened]).toEqual(["/w", "/w/src"]);
    expect([...expanded]).toEqual(["/w"]);
    expect([...toggleExpanded(opened, "/w/src")]).toEqual(["/w"]);
  });
});
