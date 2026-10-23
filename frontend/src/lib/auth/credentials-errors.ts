import { CredentialsSignin } from "next-auth";

export const CREDENTIAL_ERROR_CODES = {
  MFA_REQUIRED: "MFA_REQUIRED",
  INVALID_MFA_CODE: "INVALID_MFA_CODE",
  MFA_NOT_ENROLLED: "MFA_NOT_ENROLLED",
  NO_TENANT_MEMBERSHIP: "NO_TENANT_MEMBERSHIP",
  ACCOUNT_DEACTIVATED: "ACCOUNT_DEACTIVATED",
  INVALID_EMAIL_CODE: "INVALID_EMAIL_CODE",
  RATE_LIMITED: "RATE_LIMITED",
} as const;

export type CredentialErrorCode =
  (typeof CREDENTIAL_ERROR_CODES)[keyof typeof CREDENTIAL_ERROR_CODES];

export class MFARequiredError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.MFA_REQUIRED;
}

export class InvalidMFACodeError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.INVALID_MFA_CODE;
}

export class MFANotEnrolledError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.MFA_NOT_ENROLLED;
}

export class NoTenantMembershipError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.NO_TENANT_MEMBERSHIP;
}

export class AccountDeactivatedError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.ACCOUNT_DEACTIVATED;
}

export class InvalidEmailCodeError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.INVALID_EMAIL_CODE;
}

export class RateLimitedError extends CredentialsSignin {
  code = CREDENTIAL_ERROR_CODES.RATE_LIMITED;
}

/** Maps a Go API error code to the matching NextAuth credentials error. */
export function credentialsErrorFor(
  code: string | undefined,
): CredentialsSignin | null {
  switch (code) {
    case CREDENTIAL_ERROR_CODES.MFA_REQUIRED:
      return new MFARequiredError();
    case CREDENTIAL_ERROR_CODES.INVALID_MFA_CODE:
      return new InvalidMFACodeError();
    case CREDENTIAL_ERROR_CODES.MFA_NOT_ENROLLED:
      return new MFANotEnrolledError();
    case CREDENTIAL_ERROR_CODES.NO_TENANT_MEMBERSHIP:
      return new NoTenantMembershipError();
    case CREDENTIAL_ERROR_CODES.ACCOUNT_DEACTIVATED:
      return new AccountDeactivatedError();
    case CREDENTIAL_ERROR_CODES.INVALID_EMAIL_CODE:
      return new InvalidEmailCodeError();
    case CREDENTIAL_ERROR_CODES.RATE_LIMITED:
      return new RateLimitedError();
    default:
      return null;
  }
}

export type CredentialSignInResult = {
  error?: string | null;
  code?: string | null;
  ok?: boolean;
  status?: number;
  url?: string | null;
};

export function resolveCredentialErrorCode(
  result: CredentialSignInResult,
): string | null {
  const code = result.code?.trim();
  if (code) return code;

  const error = result.error?.trim();
  if (!error) return null;

  const known = Object.values(CREDENTIAL_ERROR_CODES);
  for (const item of known) {
    if (error.includes(item)) return item;
  }
  return error;
}
