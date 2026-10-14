import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { ActionMenu } from "@lesomnus/cxz-ui";
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
          <>
            <svg
              width="24"
              height="24"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              aria-hidden="true"
            >
              <circle cx="12" cy="12" r="10" />
              <circle cx="12" cy="9" r="3" />
              <path d="M5 19a7 7 0 0 1 14 0" />
            </svg>
            <span>{t("Profile")}</span>
          </>
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
