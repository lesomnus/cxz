import type { Session } from "#gen/cxz/session_pb";
import { AgentIdentity } from "#src/shared/components/agent-identity.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";

export function SessionIdentity({
  session,
  heading = false,
}: {
  session?: Session;
  heading?: boolean;
}) {
  useLocale();
  return (
    <AgentIdentity
      agent={session?.agent ?? ""}
      name={
        session?.name || session?.alias || session?.runtimeId || t("Session")
      }
      alias={session?.alias || session?.runtimeId || ""}
      heading={heading}
    />
  );
}
