import { errorInfos } from "#src/shared/api/error-info.ts";

// Decode the standard google.rpc.ErrorInfo detail, not prose that happens to
// mention login. Only the runtime's explicit missing-profile signal can retry
// a creation. Other authentication failures remain errors.
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
      metadata.account === account &&
      key === creationKey
    )
      return { account, key };
  }
}
