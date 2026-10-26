// Absolute paths and local file URIs name files inside the session's container.
// Network-path URLs and file URIs with remote hosts are not container paths.
export function workspaceFilePath(href: string): string | undefined {
  let path = href.split(/[?#]/, 1)[0]!;
  if (/^file:/i.test(href)) {
    try {
      const url = new URL(href);
      if (url.hostname && url.hostname !== "localhost") return;
      path = url.pathname;
    } catch {
      return;
    }
  }
  if (!path.startsWith("/") || /^\/[\/\\]/.test(path)) return;
  // Strip source references before decoding, preserving literal encoded colons.
  path = path.replace(/:[1-9]\d*(?::[1-9]\d*)?$/, "");
  try {
    const decoded = decodeURIComponent(path);
    return decoded.includes("\0") ? undefined : decoded;
  } catch {
    return path;
  }
}
