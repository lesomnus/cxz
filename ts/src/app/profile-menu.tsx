import { Icon } from "#src/shared/icons/icon.tsx";
import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { ActionMenu } from "@lesomnus/nox";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import {
  clientErrors,
  type ClientErrors,
} from "#src/shared/errors/client-errors.ts";

export function ProfileMenu({
  logout,
  exitLabel,
  openSettings,
  errors = clientErrors(),
}: {
  logout: () => Promise<void>;
  exitLabel: string;
  openSettings: () => void;
  errors?: ClientErrors;
}) {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  async function signOut() {
    if (busy) return;
    setBusy(true);
    try {
      await logout();
      errors.resolve("http:auth/logout");
    } catch (e) {
      errors.report("http:auth/logout", e, {
        source: "http",
        origin: { kind: "http", method: "POST", path: "/auth/logout" },
        method: "POST /auth/logout",
      });
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="profile-menu">
      <ActionMenu
        label={t("Profile")}
        placement="right"
        triggerClassName="resource-link profile-trigger"
        triggerContent={
          <span className="profile-avatar" aria-hidden="true">
            <Icon name="profile" color="white" width="36" height="36" />
          </span>
        }
        items={[
          {
            label: t("Settings"),
            icon: <ResourceIcon kind="settings" />,
            run: () => {
              openSettings();
              void navigate({ to: "/setting/general" });
            },
          },
          {
            label: exitLabel,
            disabled: busy,
            icon: (
              <Icon name="sign-out" width="20" height="20" aria-hidden="true" />
            ),
            run: () => void signOut(),
          },
        ]}
      />
    </div>
  );
}
