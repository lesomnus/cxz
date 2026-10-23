import { ResourceValue as ValueMenu } from "#src/shared/components/resource-value.tsx";
import { useEffect, useState } from "react";
import { Button } from "@lesomnus/nox";
import { AuxKind, type AuxProfile } from "#gen/cxz/project_svc_pb";
import { create } from "@bufbuild/protobuf";
import { AuxProfileSchema } from "#gen/cxz/project_svc_pb";
import { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import { accountRef, listAccounts } from "#src/features/accounts/account-data.ts";
import { LoginPanel } from "#src/features/accounts/login-panel.tsx";
import { auxKinds } from "./auxiliary-controls";
import styles from "#src/shared/components/resource-surface.module.css";
export function AuxiliarySettings({ c }: { c: Connection }) {
  const configuration = useRemote(
    (signal) => c.projects.auxConfig({}, { signal, timeoutMs: 10000 }),
    [c],
  );
  const accounts = useRemote((signal) => listAccounts(c, signal), [c]);
  const [profiles, setProfiles] = useState<AuxProfile[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (configuration.data)
      setProfiles(
        auxKinds.map(
          ({ kind }) =>
            configuration.data!.profiles.find((p) => p.kind === kind) ??
            create(AuxProfileSchema, { kind }),
        ),
      );
  }, [configuration.data]);
  function update(kind: AuxKind, patch: Partial<AuxProfile>) {
    setProfiles((old) =>
      old.map((p) => (p.kind === kind ? { ...p, ...patch } : p)),
    );
  }
  return (
    <div className={styles.content}>
      <p className={styles.muted}>
        Choose the account and model for each task. Session preferences control
        whether these tasks run automatically.
      </p>
      {(error || configuration.error || accounts.error) && (
        <p role="alert">{error || configuration.error || accounts.error}</p>
      )}
      {profiles.map((profile) => (
        <AuxiliaryProfile
          key={profile.kind}
          c={c}
          profile={profile}
          accounts={accounts.data ?? []}
          disabled={busy}
          update={(patch) => update(profile.kind, patch)}
        />
      ))}
      <Button
        disabled={busy || !profiles.length}
        onClick={async () => {
          setBusy(true);
          setError("");
          try {
            const saved = await c.projects.auxSetConfig(
              { profiles },
              { timeoutMs: 20000 },
            );
            setProfiles(saved.profiles);
          } catch (error) {
            setError(String(error));
          } finally {
            setBusy(false);
          }
        }}
      >
        Save configuration
      </Button>
    </div>
  );
}
function AuxiliaryProfile({
  c,
  profile,
  accounts,
  disabled,
  update,
}: {
  c: Connection;
  profile: AuxProfile;
  accounts: Awaited<ReturnType<typeof listAccounts>>;
  disabled: boolean;
  update: (patch: Partial<AuxProfile>) => void;
}) {
  const [login, setLogin] = useState(false);
  const models = useRemote(
    async (signal) =>
      profile.account
        ? c.accounts.models(accountRef(profile.account), {
            signal,
            timeoutMs: 20000,
          })
        : undefined,
    [c, profile.account],
  );
  const model = models.data?.models.find((m) => m.id === profile.model);
  return (
    <section className={styles.section}>
      <div className={styles.row}>
        <strong>{auxKinds.find((k) => k.kind === profile.kind)?.label}</strong>
        <label>
          <input
            type="checkbox"
            checked={profile.enabled}
            disabled={disabled}
            onChange={(e) => update({ enabled: e.target.checked })}
          />{" "}
          Enabled
        </label>
      </div>
      <ValueMenu
        label="Account"
        disabled={disabled || login}
        value={profile.account}
        options={[
          { value: "", label: "Choose an account" },
          ...accounts.map((a) => ({
            value: a.alias,
            label: `${a.alias} · ${a.agent}`,
          })),
        ]}
        choose={(account) => update({ account, model: "", effort: "" })}
      />
      <ValueMenu
        label="Model"
        disabled={disabled}
        value={profile.model}
        options={[
          { value: "", label: "Provider default" },
          ...(models.data?.models ?? []).map((m) => ({
            value: m.id,
            label: m.name || m.id,
          })),
          ...(!model && profile.model
            ? [{ value: profile.model, label: profile.model }]
            : []),
        ]}
        choose={(model) => update({ model, effort: "" })}
      />
      <ValueMenu
        label="Effort"
        disabled={disabled}
        value={profile.effort}
        options={[
          { value: "", label: "Provider default" },
          ...(model?.efforts ?? []).map((value) => ({ value, label: value })),
        ]}
        choose={(effort) => update({ effort })}
      />
      {models.error && <p role="alert">{models.error}</p>}
      {models.data?.needsLogin && (
        <p className={styles.muted}>This profile needs a login.</p>
      )}
      {profile.account && (
        <Button onClick={() => setLogin(!login)}>
          Log in to helper profile
        </Button>
      )}
      {login && (
        <LoginPanel
          c={c}
          target={{ account: profile.account }}
          complete={() => {
            setLogin(false);
            models.reload();
          }}
          cancel={() => setLogin(false)}
        />
      )}
    </section>
  );
}
