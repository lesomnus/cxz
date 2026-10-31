import type { Connection } from "#src/shared/api/connection.ts";
import type { Session } from "#gen/cxz/session_pb";
import type { Project } from "#gen/cxz/project_pb";
import { AccountsView } from "#src/features/accounts/accounts-view.tsx";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { useNavigate } from "@tanstack/react-router";
import { accountLocation } from "#src/shared/navigation/resource-route.ts";
export function AccountsPage({
  c,
  sessions,
  projects = [],
  sessionsLoading,
}: {
  c: Connection;
  sessions: Session[];
  projects?: Project[];
  sessionsLoading?: boolean;
}) {
  const navigate = useNavigate();
  const { account } = useWorkspaceRoute();
  return (
    <AccountsView
      c={c}
      sessions={sessions}
      projects={projects}
      sessionsLoading={sessionsLoading}
      selected={account}
      select={(alias) => void navigate(accountLocation(alias))}
    />
  );
}
