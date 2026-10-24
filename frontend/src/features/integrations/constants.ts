import type { IntegrationJobStatus } from "@/features/integrations/types";

type Tone = "default" | "success" | "warning" | "danger";

/** Label of a code through a key map; unknown codes show as they are. */
export function labelOf(
  t: (key: string) => string,
  map: Record<string, string>,
  code: string,
): string {
  const key = map[code];
  return key ? t(key) : code;
}

export const INTEGRATION_FIELD_LABEL_KEYS: Record<string, string> = {
  sender: "integrations.fields.sender",
  api_key: "integrations.fields.api_key",
  secret_key: "integrations.fields.secret_key",
  username: "integrations.fields.username",
  password: "integrations.fields.password",
  merchant_id: "integrations.fields.merchant_id",
  merchant_key: "integrations.fields.merchant_key",
  merchant_salt: "integrations.fields.merchant_salt",
};

export const INTEGRATION_PROVIDER_LABEL_KEYS: Record<string, string> = {
  log: "integrations.providers.log",
  testpay: "integrations.providers.testpay",
  paytr: "integrations.providers.paytr",
};

export const INTEGRATION_KIND_LABEL_KEYS: Record<string, string> = {
  sms: "integrations.kinds.sms",
  whatsapp: "integrations.kinds.whatsapp",
  email: "integrations.kinds.email",
  payment: "integrations.kinds.payment",
};

/** Connect form hint per provider (where the organization finds its keys). */
export const INTEGRATION_PROVIDER_HINT_KEYS: Record<string, string> = {
  log: "integrations.provider_hints.log",
  testpay: "integrations.provider_hints.testpay",
  paytr: "integrations.provider_hints.paytr",
};

/** Live list resource of integration jobs (backend jobs usecase). */
export const INTEGRATION_JOBS_RESOURCE = "tenant.integration_jobs" as const;

export const INTEGRATION_JOB_STATUSES: IntegrationJobStatus[] = [
  "pending",
  "processing",
  "submitted",
  "succeeded",
  "failed",
  "needs_verification",
  "blocked",
  "waiting_dependency",
  "reported_manually",
  "reconciled",
  "ignored",
];

export const INTEGRATION_JOB_STATUS_LABEL_KEYS: Record<
  IntegrationJobStatus,
  string
> = {
  pending: "integrations.jobs.statuses.pending",
  processing: "integrations.jobs.statuses.processing",
  submitted: "integrations.jobs.statuses.submitted",
  succeeded: "integrations.jobs.statuses.succeeded",
  failed: "integrations.jobs.statuses.failed",
  needs_verification: "integrations.jobs.statuses.needs_verification",
  ignored: "integrations.jobs.statuses.ignored",
  blocked: "integrations.jobs.statuses.blocked",
  waiting_dependency: "integrations.jobs.statuses.waiting_dependency",
  reported_manually: "integrations.jobs.statuses.reported_manually",
  reconciled: "integrations.jobs.statuses.reconciled",
};

export const INTEGRATION_JOB_STATUS_TONES: Record<IntegrationJobStatus, Tone> =
  {
    pending: "warning",
    processing: "warning",
    submitted: "warning",
    succeeded: "success",
    failed: "danger",
    needs_verification: "danger",
    ignored: "default",
    blocked: "danger",
    waiting_dependency: "warning",
    reported_manually: "success",
    reconciled: "success",
  };

export const INTEGRATION_JOB_KIND_LABEL_KEYS: Record<string, string> = {
  "sms.send": "integrations.jobs.kinds.sms_send",
  "whatsapp.send": "integrations.jobs.kinds.whatsapp_send",
  "email.send": "integrations.jobs.kinds.email_send",
  "payment.refund": "integrations.jobs.kinds.payment_refund",
  "payment.webhook": "integrations.jobs.kinds.payment_webhook",
};

export const ATTEMPT_OPERATION_LABEL_KEYS: Record<string, string> = {
  submit: "integrations.jobs.operations.submit",
  check_status: "integrations.jobs.operations.check_status",
};

export const ATTEMPT_TRIGGER_LABEL_KEYS: Record<string, string> = {
  worker: "integrations.jobs.triggers.worker",
  sweep: "integrations.jobs.triggers.sweep",
  retry: "integrations.jobs.triggers.retry",
  inline: "integrations.jobs.triggers.inline",
};

export const ATTEMPT_OUTCOME_LABEL_KEYS: Record<string, string> = {
  succeeded: "integrations.jobs.outcomes.succeeded",
  accepted: "integrations.jobs.outcomes.accepted",
  pending: "integrations.jobs.outcomes.pending",
  failed: "integrations.jobs.outcomes.failed",
  not_found: "integrations.jobs.outcomes.not_found",
  uncertain: "integrations.jobs.outcomes.uncertain",
  unsupported: "integrations.jobs.outcomes.unsupported",
};

export const ATTEMPT_OUTCOME_TONES: Record<string, Tone> = {
  succeeded: "success",
  accepted: "success",
  pending: "warning",
  failed: "danger",
  not_found: "default",
  uncertain: "danger",
  unsupported: "danger",
};

export const CHECK_STATUS_LABEL_KEYS: Record<string, string> = {
  "": "integrations.check.never",
  ok: "integrations.check.ok",
  failed: "integrations.check.failed",
};
