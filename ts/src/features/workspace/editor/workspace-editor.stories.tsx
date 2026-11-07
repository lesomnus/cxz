import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { create } from "@bufbuild/protobuf";
import { Button } from "@lesomnus/nox";
import { ProjectSchema } from "#gen/cxz/project_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { projectId } from "#src/shared/api/project-id.ts";
import { projectFixtureId } from "#src/storybook/project-id.ts";
import { EditorState, WorkspaceEditor } from "./workspace-editor";
import styles from "./workspace-editor.stories.module.css";

const sources = {
  "/workspace/README.md":
    "# Workspace preview\n\nOpen files from the explorer or manage tabs from the menu.\n",
  "/workspace/src/main.ts":
    "export const greeting = 'Hello, workspace';\nconsole.log(greeting);\n",
  "/workspace/package.json":
    '{\n  "name": "workspace-preview",\n  "private": true\n}\n',
};
function fixture(manyTabs: boolean, empty: boolean) {
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
  if (manyTabs)
    for (let i = 0; i < 16; i++)
      files.push({
        path: `/workspace/src/example-component-${i + 1}.tsx`,
        text: `export const example = ${i + 1};\n`,
      });
  state.update({ files, active: files[0]?.path ?? "" });
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
              ]
            : [{ name: "main.ts", directory: false }];
        yield { entries };
      },
      async *download({ path }: { path: string }) {
        // A deliberately late response lets the preview exercise closing during load.
        if (path.endsWith("slow.txt"))
          await new Promise((resolve) => setTimeout(resolve, 1500));
        const text =
          sources[path as keyof typeof sources] ?? "A delayed file preview.\n";
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

function Preview({ manyTabs = false, empty = false, drawer = false }) {
  const [data] = useState(() => fixture(manyTabs, empty));
  const [open, setOpen] = useState(true);
  const editor = (
    <WorkspaceEditor
      {...data}
      closePreview={drawer ? () => setOpen(false) : undefined}
    />
  );
  return drawer ? (
    <div className={`session-workspace ${styles.drawer}`} data-file-open={open}>
      <div className={styles.conversation}>
        <p>The file preview slides over this conversation on narrow screens.</p>
        <Button onClick={() => setOpen(true)}>Open file preview</Button>
      </div>
      <div className="workspace-editor-layer">{editor}</div>
    </div>
  ) : (
    <div className={styles.editor}>{editor}</div>
  );
}

const meta = {
  title: "Workspace/File editor",
  component: Preview,
  args: { manyTabs: false, empty: false, drawer: false },
  parameters: {
    layout: "fullscreen",
    docs: {
      description: {
        component:
          "The file explorer and viewer share one outer background. Viewer padding surrounds a rounded content surface containing the tab bar and Monaco area, both on the editor background. Section radius and compact inset tokens match resource pages. The status strip belongs to the viewer outside content. The tab menu copies the active container path, closes all/other tabs, and retains Connect/Disconnect. The explorer separator still resizes on drag or keyboard focus. This preview uses real Monaco with simulated directory/download requests; slow.txt deliberately finishes late so tab closure can be checked during loading. No server is contacted.",
      },
    },
  },
  tags: ["autodocs"],
} satisfies Meta<typeof Preview>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Tabs: Story = {};
export const OverflowingTabs: Story = { args: { manyTabs: true } };
export const Empty: Story = { args: { empty: true } };
export const NarrowPreview: Story = {
  args: { drawer: true },
};
