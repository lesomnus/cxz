import { useState } from "react";
import { createRoot } from "react-dom/client";
import { Button } from "@lesomnus/nox";
import { SourceEditor } from "@lesomnus/nox/editor";
import "@lesomnus/nox/styles.css";

function Preview() {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  return (
    <main>
      <textarea
        aria-label="Draft"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
      />
      <Button onClick={() => setOpen(true)}>Open editor</Button>
      <div style={{ height: 360 }}>
        {open && (
          <SourceEditor
            value="const ready = true;"
            path="main.ts"
            ariaLabel="File preview"
            readOnly
          />
        )}
      </div>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<Preview />);
