import openaiBlossom from "./assets/brands/openai-blossom.svg?raw";
import claudeLogo from "./assets/brands/claude-one-color.svg";

// Keep the official artwork intact; the Blossom source uses currentColor.
// Provenance and brand usage conditions live beside the SVG files.
export function AgentBrand({ agent }: { agent: string }) {
  if (agent !== "codex" && agent !== "claude")
    return <>{agent ? agent[0].toUpperCase() + agent.slice(1) : "Agent"}</>;
  const name = agent === "codex" ? "Codex" : "Claude";
  return (
    <span
      className={`agent-brand agent-brand-${agent}`}
      role="img"
      aria-label={name}
      title={name}
    >
      {agent === "codex" ? (
        <span
          aria-hidden="true"
          dangerouslySetInnerHTML={{ __html: openaiBlossom }}
        />
      ) : (
        <img
          src={claudeLogo}
          alt=""
          aria-hidden="true"
          width="64.092"
          height="14"
        />
      )}
    </span>
  );
}
