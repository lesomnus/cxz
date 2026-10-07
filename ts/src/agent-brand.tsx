import { t } from "./i18n";
import { useLocale } from "./i18n-react";
import openaiBlossom from "./assets/brands/openai-blossom.svg?raw";
import claudeLogo from "./assets/brands/claude-spark-white.svg";

// Blossom uses its original currentColor; Claude keeps the white Spark path
// extracted from the official lockup. Provenance/conditions accompany the SVGs.
export function AgentBrand({ agent }: { agent: string }) {
  useLocale();
  if (agent !== "codex" && agent !== "claude")
    return <>{agent ? agent[0].toUpperCase() + agent.slice(1) : t("Agent")}</>;
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
          width="14"
          height="14"
        />
      )}
    </span>
  );
}
