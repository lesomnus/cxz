import type { Connection } from "#src/shared/api/connection.ts";
import type { Session } from "#gen/cxz/session_pb";
import { AccountsView } from "#src/features/accounts/accounts-view.tsx";
import { useWorkspaceRoute } from "#src/app/router.tsx";
import { useNavigate } from "@tanstack/react-router";
export function AccountsPage({
  c,
  sessions,
  sessionsLoading,
}: {
  c: Connection;
  sessions: Session[];
  sessionsLoading?: boolean;
}) {
  const navigate = useNavigate();
  const { account } = useWorkspaceRoute();
  return (
    <AccountsView
      c={c}
      sessions={sessions}
      sessionsLoading={sessionsLoading}
      selected={account}
      select={(alias) =>
        void navigate({ to: "/accounts", search: { account: alias } })
      }
    />
  );
}
