import { MAX_UPLOAD_BYTES } from "./composer-upload";

export type PickedFile = { file: File; name: string; directory?: boolean };
export type DirectoryFile = { path: string; file?: File };
const encoder = new TextEncoder();
const MAX_ENTRIES = 10_000;

function safePath(path: string) {
  const parts = path.split("/");
  if (
    parts.some(
      (p) => !p || p === "." || p === ".." || /[\\\x00-\x1f\x7f]/.test(p),
    )
  )
    throw new Error("Invalid upload filename.");
  return path;
}

// Blob parts retain the browser's file backing; building an archive does not
// read every file into JS memory. PAX paths preserve Unicode and long names.
export function archiveDirectory(
  name: string,
  entries: DirectoryFile[],
): PickedFile {
  safePath(name);
  const parts: BlobPart[] = [];
  let size = 1024;
  const seen = new Set<string>();
  function append(path: string, body: Blob, type: string) {
    const header = new Uint8Array(512);
    function text(offset: number, value: string) {
      header.set(encoder.encode(value), offset);
    }
    function octal(offset: number, width: number, value: number) {
      text(offset, value.toString(8).padStart(width - 1, "0") + "\0");
    }
    text(0, path);
    octal(100, 8, type === "5" ? 0o755 : 0o644);
    octal(108, 8, 0);
    octal(116, 8, 0);
    octal(124, 12, body.size);
    octal(136, 12, 0);
    header.fill(32, 148, 156);
    text(156, type);
    text(257, "ustar\0");
    text(263, "00");
    text(
      148,
      header
        .reduce((sum, byte) => sum + byte, 0)
        .toString(8)
        .padStart(6, "0") + "\0 ",
    );
    const padding = (512 - (body.size % 512)) % 512;
    size += 512 + body.size + padding;
    if (size > MAX_UPLOAD_BYTES) throw new Error("Files are limited to 1 GiB.");
    parts.push(header, body, new Uint8Array(padding));
  }
  if (entries.length > MAX_ENTRIES)
    throw new Error("Folders are limited to 10,000 entries.");
  for (const [index, entry] of entries.entries()) {
    const path = safePath(entry.path);
    if (path !== name && !path.startsWith(name + "/"))
      throw new Error("Invalid upload filename.");
    if (seen.has(path)) throw new Error("Duplicate upload filename.");
    seen.add(path);
    const archivePath = path + (entry.file ? "" : "/");
    let headerPath = archivePath;
    if (
      encoder.encode(archivePath).length > 100 ||
      /[^\x20-\x7e]/.test(archivePath)
    ) {
      const record = `path=${archivePath}\n`;
      let length = encoder.encode(record).length + 3;
      while (
        length !==
        encoder.encode(record).length + String(length).length + 1
      )
        length = encoder.encode(record).length + String(length).length + 1;
      append(`PaxHeaders/${index}`, new Blob([`${length} ${record}`]), "x");
      headerPath = `entry-${index}`;
    }
    append(headerPath, entry.file ?? new Blob(), entry.file ? "0" : "5");
  }
  parts.push(new Uint8Array(1024));
  return {
    name,
    directory: true,
    file: new File(parts, `${name}.tar`, { type: "application/x-tar" }),
  };
}

export function pickedFiles(files: readonly File[]): PickedFile[] {
  const groups = new Map<string, DirectoryFile[]>();
  const result: PickedFile[] = [];
  for (const file of files) {
    const path = file.webkitRelativePath;
    if (!path) {
      result.push({ file, name: file.name });
      continue;
    }
    const name = path.split("/")[0];
    const group = groups.get(name) ?? [{ path: name }];
    group.push({ path, file });
    groups.set(name, group);
  }
  for (const [name, entries] of groups)
    result.push(archiveDirectory(name, entries));
  return result;
}

async function directoryFiles(root: FileSystemEntry): Promise<DirectoryFile[]> {
  const result: DirectoryFile[] = [];
  let total = 0;
  async function walk(entry: FileSystemEntry, path: string, depth: number) {
    if (result.length >= MAX_ENTRIES || depth > 128)
      throw new Error("Folders are limited to 10,000 entries.");
    safePath(path);
    if (entry.isFile) {
      const file = await new Promise<File>((resolve, reject) =>
        (entry as FileSystemFileEntry).file(resolve, reject),
      );
      total += file.size;
      if (total > MAX_UPLOAD_BYTES)
        throw new Error("Files are limited to 1 GiB.");
      result.push({ path, file });
    } else if (entry.isDirectory) {
      result.push({ path });
      const reader = (entry as FileSystemDirectoryEntry).createReader();
      // Chromium returns pages, often 100 entries. Drain through the empty page.
      for (;;) {
        const children = await new Promise<FileSystemEntry[]>(
          (resolve, reject) => reader.readEntries(resolve, reject),
        );
        if (!children.length) break;
        for (const child of children)
          await walk(child, `${path}/${child.name}`, depth + 1);
      }
    }
  }
  await walk(root, root.name, 0);
  return result;
}

export function droppedFiles(data: DataTransfer): Promise<PickedFile[]> {
  // Capture entries/files synchronously: drag data becomes unreadable after
  // the drop event returns, even while an async directory traversal continues.
  const items = Array.from(data.items)
    .filter((item) => item.kind === "file")
    .map((item) => {
      const compatible = item as DataTransferItem & {
        getAsEntry?: () => FileSystemEntry | null;
      };
      return {
        entry: compatible.getAsEntry?.() ?? item.webkitGetAsEntry?.(),
        file: item.getAsFile(),
      };
    });
  if (!items.length)
    return Promise.resolve(pickedFiles(Array.from(data.files)));
  return (async () => {
    const result: PickedFile[] = [];
    for (const { entry, file } of items) {
      if (entry?.isDirectory)
        result.push(archiveDirectory(entry.name, await directoryFiles(entry)));
      else if (file) result.push({ file, name: file.name });
    }
    return result;
  })();
}
