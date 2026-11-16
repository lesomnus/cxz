import type { ProjectPathEntry } from "#gen/cxz/project_svc_pb";

// The file explorer's listings belong to the project, not to the pane that
// happens to be showing them. They used to be component state, so switching
// sessions or closing the editor threw the tree away: coming back listed the
// workspace root again, collapsed, with everything below it gone.
//
// A cache of a directory is only ever a claim about the past, so expanding a
// directory still asks for it -- the entries already held are what the tree
// renders while that answer is on its way.

// How many directories a project keeps. Each may hold up to 2048 entries, and
// a person who has been working in a tree all day has expanded a few dozen of
// them, not a few hundred.
export const directoryCacheLimit = 256;

/** The listings to keep, with path's entries recorded. */
export function cacheDirectory(
  current: ReadonlyMap<string, ProjectPathEntry[]>,
  path: string,
  entries: ProjectPathEntry[],
  keep: { expanded: ReadonlySet<string>; root: string; limit?: number },
): Map<string, ProjectPathEntry[]> {
  const next = new Map(current);
  next.delete(path);
  next.set(path, entries);
  const limit = keep.limit ?? directoryCacheLimit;
  if (next.size <= limit) return next;
  // Oldest first, and only what is not on screen. The tree renders from this
  // map and asks again only when a directory is expanded, so dropping an
  // expanded one would leave it looking empty with nothing to correct it.
  for (const candidate of next.keys()) {
    if (next.size <= limit) break;
    if (candidate === path || candidate === keep.root) continue;
    if (keep.expanded.has(candidate)) continue;
    next.delete(candidate);
  }
  return next;
}

export function toggleExpanded(
  current: ReadonlySet<string>,
  path: string,
): Set<string> {
  const next = new Set(current);
  if (!next.delete(path)) next.add(path);
  return next;
}

/**
 * The directories a reopened pane shows, so that it asks for those again
 * rather than for every directory the project has ever listed.
 */
export function visibleDirectories(
  directories: ReadonlyMap<string, unknown>,
  expanded: ReadonlySet<string>,
  root: string,
): string[] {
  if (!root) return [];
  const visible = [root];
  // The tree builds a child's path from a root without its trailing slash, so
  // a parent is compared against the same thing.
  const top = root.length > 1 ? root.replace(/\/$/, "") : root;
  for (const path of expanded) {
    if (path === root) continue;
    // Below a collapsed ancestor is not on screen, however expanded it is.
    if (!directories.has(path)) continue;
    let parent = path.slice(0, path.lastIndexOf("/"));
    if (parent === "") parent = "/";
    if (parent !== top && !expanded.has(parent)) continue;
    visible.push(path);
  }
  return visible;
}
