import { AgentBrand } from "./agent-brand";

export function AgentIdentity({
  agent,
  name,
  alias,
  heading = false,
}: {
  agent: string;
  name: string;
  alias: string;
  heading?: boolean;
}) {
  const Title = heading ? "strong" : "span";
  return (
    <span className="session-heading">
      <AgentBrand agent={agent} />
      <span className="session-label">
        <Title className="session-title">{name}</Title>
        <span className="session-description">
          <span className="session-alias">{alias}</span>
        </span>
      </span>
    </span>
  );
}
