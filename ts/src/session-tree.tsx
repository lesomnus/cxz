import { useEffect, useRef, useState } from "react";
import { useQuery } from "@lesomnus/payday/react";
import type { Project } from "../gen/cxz/project_pb";
import type { Session } from "../gen/cxz/session_pb";
import { SessionService } from "../gen/cxz/session_svc_pb";
import { ref } from "./connection";
import { Button } from "./button";
import { AgentBrand } from "./agent-brand";

export function SessionTreeGroup({
  project,
  selected,
  open,
  toggle,
  select,
}: {
  project: Project;
  selected: string;
  open: boolean;
  toggle: () => void;
  select: (id: string) => void;
}) {
  const [after, setAfter] = useState("");
  const [items, setItems] = useState<Session[]>([]);
  const pages = useRef(new Map<string, Session[]>());
  const query = useQuery(SessionService.method.list, {
    filters: [{ listed: true, project: ref(project.runtimeId) }],
    size: 50,
    after,
  });
  useEffect(() => {
    if (!query.data) return;
    pages.current.set(after, query.data.items);
    setItems([
      ...new Map(
        [...pages.current.values()].flat().map((s) => [s.runtimeId, s]),
      ).values(),
    ]);
  }, [query.data, after]);
  return (
    <section
      className="session-tree-group"
      aria-label={project.name || project.alias}
    >
      <Button className="tree-project" aria-expanded={open} onClick={toggle}>
        <span>{project.name || project.alias}</span>
      </Button>
      {open && (
        <div className="tree-sessions">
          {items.map((s) => (
            <Button
              key={s.runtimeId}
              className={`tree-session ${selected === s.runtimeId ? "active" : ""}`}
              pressTarget=".session-heading"
              aria-current={selected === s.runtimeId ? "true" : undefined}
              onClick={() => select(s.runtimeId)}
            >
              <SessionIndicator session={s} />
              <span className="session-label">
                <span className="session-heading">
                  <AgentBrand agent={s.agent} />
                  <span className="session-title">
                    {s.name || s.alias || s.runtimeId}
                  </span>
                </span>
                <span className="session-description">
                  <span className="session-alias">
                    {s.alias || s.runtimeId}
                  </span>
                  {s.model && <span className="session-model">{s.model}</span>}
                </span>
              </span>
            </Button>
          ))}
          {query.state === "pending" && !query.data && (
            <small className="tree-message">Loading…</small>
          )}
          {query.data && !items.length && (
            <small className="tree-message">No sessions</small>
          )}
          {!!query.error && <small role="alert">{String(query.error)}</small>}
          {query.data?.next && (
            <Button
              className="tree-more"
              onClick={() => setAfter(query.data!.next)}
            >
              More sessions
            </Button>
          )}
        </div>
      )}
    </section>
  );
}

function SessionIndicator({ session }: { session: Session }) {
  const [frame, setFrame] = useState(0);
  const state = session.status?.state || "unknown";
  const question = !!session.status?.pending.length;
  const working = ["running", "working", "waiting_input"].includes(state);
  useEffect(() => {
    if (!working) return;
    const timer = setInterval(() => setFrame((v) => (v + 1) % 8), 200);
    return () => clearInterval(timer);
  }, [working]);
  // Match the TUI's question and braille activity indicators, in monochrome.
  const symbol = question
    ? "?"
    : working
      ? [..."⣟⣯⣷⣾⣽⣻⢿⡿"][frame]
      : state === "stopped"
        ? "○"
        : " ";
  return (
    <span
      className={`session-indicator ${question ? "attention" : ""}`}
      role="img"
      aria-label={question ? "Awaiting answer" : state}
    >
      {symbol}
    </span>
  );
}
