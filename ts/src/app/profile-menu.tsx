import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { ActionMenu } from "@lesomnus/nox";
import { ResourceIcon } from "#src/shared/navigation/resource-icon.tsx";
import { t } from "#src/shared/i18n/i18n.ts";

export function ProfileMenu({
  logout,
  exitLabel,
  openSettings,
}: {
  logout: () => Promise<void>;
  exitLabel: string;
  openSettings: () => void;
}) {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function signOut() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await logout();
    } catch (e) {
      setError(String(e));
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
            <svg width="36" height="36" viewBox="0 0 36 36" fill="none">
              <circle cx="18" cy="18" r="8" stroke="white" strokeWidth="1.5" />
              <circle cx="18" cy="18" r="2" fill="white" />
              <circle cx="18" cy="10" r="2" fill="white" />
            </svg>
          </span>
        }
        items={[
          {
            label: t("Settings"),
            icon: <ResourceIcon kind="settings" />,
            run: () => {
              openSettings();
              void navigate({ to: "/settings/general" });
            },
          },
          {
            label: exitLabel,
            disabled: busy,
            icon: (
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                aria-hidden="true"
              >
                <path d="M10 4H4v16h6M8 12h13m-5-5 5 5-5 5" />
              </svg>
            ),
            run: () => void signOut(),
          },
        ]}
      />
      {error && (
        <p className="profile-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
