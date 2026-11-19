import { errorInfos } from "#src/shared/api/error-info.ts";

// Match the runtime's missing-profile challenge to the session being started.
export function requiredSessionLogin(
  error: unknown,
  account: string,
  creationKey: string,
) {
  for (const { reason, domain, metadata } of errorInfos(error)) {
    const key = metadata.session_key || creationKey;
    if (
      reason === "PROJECT_LOGIN_REQUIRED" &&
      domain === "cxz.auth" &&
      metadata.account &&
      metadata.account === account &&
      key === creationKey
    )
      return { account: metadata.account, key };
  }
}
