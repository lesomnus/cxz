import type { Session } from "../gen/cxz/session_pb";
import { AgentBrand } from "./agent-brand";
import { t } from "./i18n";
import { useLocale } from "./i18n-react";

export function SessionIdentity({
  session,
  heading = false,
}: {
  session?: Session;
  heading?: boolean;
}) {
  useLocale();
  const Title = heading ? "strong" : "span";
  return (
    <span className="session-heading">
      <AgentBrand agent={session?.agent ?? ""} />
      <span className="session-label">
        <Title className="session-title">
          {session?.name ||
            session?.alias ||
            session?.runtimeId ||
            t("Session")}
        </Title>
        <span className="session-description">
          <span className="session-alias">
            {session?.alias || session?.runtimeId}
          </span>
        </span>
      </span>
    </span>
  );
}
