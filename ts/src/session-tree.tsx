import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import { useEffect, useState } from "react";
import type { Project } from "../gen/cxz/project_pb";
import type { Session } from "../gen/cxz/session_pb";
import { Button } from "./button";
import { AgentBrand } from "./agent-brand";

export function SessionTreeGroup({
  project,
  items,
  loading,
  selected,
  open,
  toggle,
  select,
}: {
  project: Project;
  items: Session[];
  loading: boolean;
  selected: string;
  open: boolean;
  toggle: () => void;
  select: (id: string) => void;
}) {
  useLocale();
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
          {loading && !items.length && (
            <small className="tree-message">{t("Loading…")}</small>
          )}
          {!loading && !items.length && (
            <small className="tree-message">{t("No sessions")}</small>
          )}
        </div>
      )}
    </section>
  );
}

function SessionIndicator({ session }: { session: Session }) {
  useLocale();
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
      aria-label={question ? t("Awaiting answer") : state}
    >
      {symbol}
    </span>
  );
}
