import type { LucideIcon } from "lucide-react";

/** Bulk action contract from resource meta API (no icon). */
export type BulkActionMeta = {
  id: string;
  label_key: string;
  permission: string;
  destructive?: boolean;
  reversible?: boolean;
  confirm_key?: string;
};

/** Bulk action for UI — icon is required. */
export type BulkActionDef = BulkActionMeta & {
  icon: LucideIcon;
};

export type BulkTarget = {
  scope: "ids" | "query";
  ids?: string[];
  query?: Record<string, string>;
  /** Arguments of parameterised actions (e.g. the new status). */
  params?: Record<string, string>;
};

/** One skipped or failed item of a bulk run (the first 200 are listed). */
export type BulkIssue = {
  entity_uuid: string;
  label?: string;
  /** Reason code, e.g. `invalid_transition`, `stale`, `not_found`. */
  error: string;
  skipped?: boolean;
};

export type BulkSummary = {
  total: number;
  succeeded: number;
  failed: number;
  /** Left as they were: the action does not apply (not in `failed`). */
  skipped?: number;
  issues?: BulkIssue[];
};

export type BulkJob = {
  uuid: string;
  resource: string;
  action: string;
  status: string;
  target?: BulkTarget;
  summary?: BulkSummary;
  error?: string | null;
  rollback_until?: string | null;
  applied_at?: string | null;
  created_at: string;
};

export type BulkExecuteSyncResult = {
  sync: true;
  summary: BulkSummary;
};

export type BulkResource =
  | "platform.users"
  | "platform.roles"
  | "tenant.branches"
  | "tenant.staff"
  | "tenant.resources"
  | "tenant.services"
  | "tenant.service_bundles"
  | "tenant.products"
  | "tenant.customers"
  | "tenant.reservations"
  | "tenant.cash_accounts"
  | "tenant.session_packages"
  | "platform.organizations";

/**
 * Execute route per resource. `tenant.*` resources are organization-scoped:
 * call them with a tenant `ApiScope` (see `useApiScope`).
 */
export const BULK_PATHS: Record<BulkResource, string> = {
  "platform.users": "/v1/platform/users/bulk",
  "platform.roles": "/v1/platform/roles/bulk",
  "platform.organizations": "/v1/platform/organizations/bulk",
  "tenant.branches": "/v1/tenant/branches/bulk",
  "tenant.staff": "/v1/tenant/staff/bulk",
  "tenant.resources": "/v1/tenant/resources/bulk",
  "tenant.services": "/v1/tenant/services/bulk",
  "tenant.service_bundles": "/v1/tenant/service-bundles/bulk",
  "tenant.products": "/v1/tenant/products/bulk",
  "tenant.customers": "/v1/tenant/customers/bulk",
  "tenant.reservations": "/v1/tenant/reservations/bulk",
  "tenant.cash_accounts": "/v1/tenant/cash/accounts/bulk",
  "tenant.session_packages": "/v1/tenant/session-packages/bulk",
};

export type SelectionScope =
  | { mode: "none" }
  | { mode: "page"; ids: string[] }
  | { mode: "all"; query: Record<string, string>; total: number };

export type ListJobsResult = {
  items: BulkJob[];
  total: number;
  limit: number;
  offset: number;
};
