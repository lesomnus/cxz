import { readdir, readFile, writeFile } from "node:fs/promises";
const directory = new URL("../dist/", import.meta.url);
for (const entry of await readdir(directory, { recursive: true })) {
  if (!entry.endsWith(".d.ts")) continue;
  const file = new URL(entry, directory);
  let content = await readFile(file, "utf8");
  // Styles have a public entry; declarations require no CSS loader.
  content = content.replace(/^import "\.\/tokens\.css";\r?\n/m, "");
  // Explicit extensions support both Bundler and NodeNext consumers.
  content = content.replace(
    /(from\s+["'])(\.{1,2}\/[^"']+)(["'])/g,
    (match, prefix, path, suffix) =>
      path.endsWith(".js") ? match : `${prefix}${path}.js${suffix}`,
  );
  await writeFile(file, content);
}
