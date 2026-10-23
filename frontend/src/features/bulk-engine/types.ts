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

export type BulkSummary = {
  total: number;
  succeeded: number;
  failed: number;
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
  | "tenant.customers"
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
  "tenant.customers": "/v1/tenant/customers/bulk",
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
