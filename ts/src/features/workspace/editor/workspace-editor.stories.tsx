import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { Button } from "@lesomnus/nox";
import { ProjectSchema } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { projectFixtureId } from "#src/storybook/project-id.ts";
import { EditorState, WorkspaceEditor } from "./workspace-editor";
import { WorkspaceEditorLayer } from "./workspace-editor-layer";
import styles from "./workspace-editor.stories.module.css";

const sources = {
  "/workspace/README.md":
    "# Workspace preview\n\nOpen files from the explorer or manage tabs from the menu.\n\n## Features\n\n- Keep a preview mode for each Markdown tab.\n- Return to the same source position.\n\n| File | View |\n| --- | --- |\n| Markdown | Rendered or source |\n| TypeScript | Source |\n\n```typescript\nexport const greeting = 'Hello, workspace';\nconsole.log(greeting);\n```\n\n[Open source](/workspace/src/main.ts)\n\n## Notes\n\nThis rendered document uses the conversation Markdown renderer.\n",
  "/workspace/src/main.ts":
    "export const greeting = 'Hello, workspace';\nconsole.log(greeting);\n",
  "/workspace/package.json":
    '{\n  "name": "workspace-preview",\n  "private": true\n}\n',
};
const syntaxSources = {
  "/workspace/script.py":
    "def greet(name):\n    # Highlight Python automatically\n    return f'Hello, {name}'\n\nprint(greet('world'))\n",
  "/workspace/Dockerfile":
    'FROM node:22-alpine\nWORKDIR /workspace\nCOPY . .\nRUN npm ci\nCMD ["npm", "start"]\n',
  "/workspace/api.proto":
    'syntax = "proto3";\npackage example;\nmessage Greeting {\n  string text = 1;\n}\n',
  "/workspace/scratch": "export const answer = 42;\nconsole.log('hello');\n",
};
function fixture(
  manyTabs: boolean,
  empty: boolean,
  rendered: boolean,
  languageExamples: boolean,
) {
  const project = create(ProjectSchema, {
    id: projectFixtureId(1),
    name: "Workspace preview",
    alias: "preview",
    status: { remoteWorkspace: "/workspace" },
  });
  const state = new EditorState();
  const files = empty
    ? []
    : Object.entries(sources).map(([path, text]) => ({ path, text }));
  if (languageExamples)
    files.push(
      ...Object.entries(syntaxSources).map(([path, text]) => ({ path, text })),
    );
  if (manyTabs)
    for (let i = 0; i < 16; i++)
      files.push({
        path: `/workspace/src/example-component-${i + 1}.tsx`,
        text: `export const example = ${i + 1};\n`,
      });
  state.update({
    files: files.map((file) => ({
      ...file,
      preview: rendered && file.path.endsWith(".md"),
    })),
    active: languageExamples ? "/workspace/script.py" : (files[0]?.path ?? ""),
  });
  const c = {
    inProcess: true,
    editors: new Map([[projectId(project), state]]),
    projects: {
      async *paths({ path }: { path: string }) {
        const entries =
          path === "/workspace"
            ? [
                { name: "README.md", directory: false },
                { name: "src", directory: true },
                { name: "package.json", directory: false },
                { name: "slow.txt", directory: false },
                ...Object.keys(syntaxSources).map((path) => ({
                  name: path.split("/").at(-1)!,
                  directory: false,
                })),
              ]
            : [{ name: "main.ts", directory: false }];
        yield { entries };
      },
      async *download({ path }: { path: string }) {
        // A deliberately late response lets the preview exercise closing during load.
        if (path.endsWith("slow.txt"))
          await new Promise((resolve) => setTimeout(resolve, 1500));
        const text =
          sources[path as keyof typeof sources] ??
          syntaxSources[path as keyof typeof syntaxSources] ??
          "A delayed file preview.\n";
        const data = new TextEncoder().encode(text);
        yield { data, totalSize: BigInt(data.length) };
      },
      async editor() {
        return { workspace: "/workspace", simulated: true };
      },
    },
  } as unknown as Connection;
  return { c, project };
}

function Preview({
  manyTabs = false,
  empty = false,
  drawer = false,
  rendered = false,
  languageExamples = false,
}) {
  const [data] = useState(() =>
    fixture(manyTabs, empty, rendered, languageExamples),
  );
  const [open, setOpen] = useState(true);
  const editor = (
    <WorkspaceEditor
      {...data}
      closePreview={drawer ? () => setOpen(false) : undefined}
    />
  );
  return drawer ? (
    <div className={`session-workspace ${styles.drawer}`} data-file-open={open}>
      <div className="session-split">
        <div className={styles.conversation}>
          <p>
            The file preview slides over this conversation on narrow screens.
          </p>
          <Button onClick={() => setOpen(true)}>Open file preview</Button>
        </div>
        <div className="session-divider" />
        <WorkspaceEditorLayer close={() => setOpen(false)}>
          {editor}
        </WorkspaceEditorLayer>
      </div>
    </div>
  ) : (
    <div className={styles.editor}>{editor}</div>
  );
}

const meta = {
  title: "Workspace/File editor",
  component: Preview,
  args: {
    manyTabs: false,
    empty: false,
    drawer: false,
    rendered: false,
    languageExamples: false,
  },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The headerless file explorer and viewer share one outer background. The transparent tab strip contains Monaco-colored tabs; the active filename is underlined and each close button follows the tab's inset and corner centers. The editor viewport keeps its Monaco background and rounded left corners even when empty. Tabs and the clickable footer path begin at the left corner's tangent; the path copies the absolute container path. The searchable syntax selector defaults to Auto, using registered filenames/extensions/shebangs; a manual language is retained per file and applied without replacing the editor/model. The eye button before the tab menu toggles rendered Markdown, remembers each file's mode, and is disabled for other files. Preview uses the conversation Markdown renderer and ScrollArea; returning to source restores the Monaco view. The tab menu copies the active container path, closes all/other tabs, and retains Connect/Disconnect. Its fixed footer shares EditorOptions with the composer: file controls persist editor.insertSpaces, editor.wordWrap and editor.showControlCharacters and update Monaco in place; the composer retains its session and composer preferences. The explorer separator occupies the centered half of the pane and appears on hover, drag or keyboard focus. This preview uses real Monaco with simulated directory/download requests; slow.txt deliberately finishes late so tab closure can be checked during loading. No server is contacted.",
      },
    },
  },
  tags: ["autodocs"],
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Tabs: Story = {};
export const SyntaxHighlighting: Story = { args: { languageExamples: true } };
export const RenderedMarkdown: Story = { args: { rendered: true } };
export const OverflowingTabs: Story = { args: { manyTabs: true } };
export const Empty: Story = { args: { empty: true } };
export const NarrowPreview: Story = {
  args: { drawer: true },
  parameters: {
    docs: {
      description: {
        story:
          "The production preview layer slides down over the conversation. Drag its bottom boundary or use the arrow keys to resize; double-click restores the initial height. The outside close button follows its lower-right corner and remains inside the conversation viewport. Closing and reopening retains the chosen height and Monaco instance. At a docked workspace width, the layer releases its layout and hides its resize/close chrome.",
      },
    },
  },
};
