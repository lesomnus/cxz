// A leading slash names a path inside the session's container. Network-path
// URLs (//host/path) still name an external site.
export function workspaceFilePath(href: string): string | undefined {
  if (!href.startsWith("/") || /^\/[\/\\]/.test(href)) return;
  const path = href.split(/[?#]/, 1)[0]!;
  try {
    const decoded = decodeURIComponent(path);
    return decoded.includes("\0") ? undefined : decoded;
  } catch {
    return path;
  }
}
