export type ExportFormat = "pdf" | "xlsx" | "csv" | "json";
export type ImportFormat = "json" | "xlsx" | "csv" | "tsv";

import type { BulkActionMeta } from "@/features/bulk-engine/types";

export type ResourceCapabilities = {
  create?: boolean;
  read?: boolean;
  update?: boolean;
  delete?: boolean;
  search?: boolean;
  filter?: boolean;
  sort?: boolean;
  export?: boolean;
  import?: boolean;
  bulk?: boolean;
};

export type ResourceMeta = {
  resource?: string;
  capabilities?: ResourceCapabilities;
  sortable?: string[];
  filterable?: string[];
  bulk_actions?: BulkActionMeta[];
};

export type ExportJob = {
  uuid: string;
  resource: string;
  format: string;
  status: string;
  row_count: number;
  error?: string | null;
  download_url?: string | null;
  created_at: string;
};

export type ImportJob = {
  uuid: string;
  resource: string;
  format: string;
  status: string;
  mapping?: Record<string, string>;
  defaults?: Record<string, string>;
  preview_summary?: ImportPreviewSummary;
  error?: string | null;
  rollback_until?: string | null;
  applied_at?: string | null;
  created_at: string;
};

/** One field an import row would change (dry-run diff adapters). */
export type ImportFieldChange = { field: string; from: string; to: string };

export type ImportPreviewRow = {
  index: number;
  data: Record<string, unknown>;
  /** Dry-run diff adapters only: what the row would do. */
  op?: "create" | "update";
  changes?: ImportFieldChange[];
};

export type ImportPreviewSummary = {
  total: number;
  valid: number;
  invalid: number;
  /** Dry-run diff adapters only. */
  creates?: number;
  updates?: number;
  rows?: ImportPreviewRow[] | null;
  errors?: { index: number; field?: string; error: string }[] | null;
};

export type AppSettings = {
  company_name: string;
  tagline: string;
  primary_color: string;
  address: string;
  phone: string;
  email: string;
  website: string;
  footer_text: string;
  paper_size: string;
  logo_url?: string | null;
};

export type ActivityEvent = {
  uuid: string;
  actor_user_id?: number | null;
  action: string;
  resource: string;
  resource_uuid?: string | null;
  payload: Record<string, unknown>;
  created_at: string;
};

/**
 * Export / import resources. `tenant.*` resources are organization-scoped and
 * go through `/v1/tenant/*` with the tenant `ApiScope` (see `useApiScope`).
 */
export type IoResource =
  | "platform.users"
  | "platform.roles"
  | "platform.notifications"
  | "platform.activity"
  | "tenant.branches"
  | "tenant.staff"
  | "tenant.resources"
  | "tenant.services"
  | "tenant.products"
  | "tenant.customers"
  | "tenant.reservations"
  | "tenant.cash_accounts"
  | "tenant.cash_movements"
  | "platform.organizations";

export const EXPORT_PATHS: Record<IoResource, string> = {
  "platform.users": "/v1/platform/users/export",
  "platform.roles": "/v1/platform/roles/export",
  "platform.notifications": "/v1/platform/notifications/export",
  "platform.activity": "/v1/platform/activity/export",
  "platform.organizations": "/v1/platform/organizations/export",
  "tenant.branches": "/v1/tenant/branches/export",
  "tenant.staff": "/v1/tenant/staff/export",
  "tenant.resources": "/v1/tenant/resources/export",
  "tenant.services": "/v1/tenant/services/export",
  "tenant.products": "/v1/tenant/products/export",
  "tenant.customers": "/v1/tenant/customers/export",
  "tenant.reservations": "/v1/tenant/reservations/export",
  "tenant.cash_accounts": "/v1/tenant/cash/accounts/export",
  "tenant.cash_movements": "/v1/tenant/cash/movements/export",
};

export const IMPORT_PATHS: Partial<
  Record<IoResource, { upload: string; sample: string }>
> = {
  "platform.users": {
    upload: "/v1/platform/users/import",
    sample: "/v1/platform/users/import/sample",
  },
  "platform.roles": {
    upload: "/v1/platform/roles/import",
    sample: "/v1/platform/roles/import/sample",
  },
  "tenant.branches": {
    upload: "/v1/tenant/branches/import",
    sample: "/v1/tenant/branches/import/sample",
  },
  "tenant.staff": {
    upload: "/v1/tenant/staff/import",
    sample: "/v1/tenant/staff/import/sample",
  },
  "tenant.resources": {
    upload: "/v1/tenant/resources/import",
    sample: "/v1/tenant/resources/import/sample",
  },
  "tenant.services": {
    upload: "/v1/tenant/services/import",
    sample: "/v1/tenant/services/import/sample",
  },
  "tenant.products": {
    upload: "/v1/tenant/products/import",
    sample: "/v1/tenant/products/import/sample",
  },
  "tenant.customers": {
    upload: "/v1/tenant/customers/import",
    sample: "/v1/tenant/customers/import/sample",
  },
};

export const IMPORT_SCHEMA: Partial<
  Record<IoResource, { key: string; labelKey: string; required?: boolean }[]>
> = {
  "platform.users": [
    { key: "email", labelKey: "users.fields.email", required: true },
    { key: "name", labelKey: "users.fields.name", required: true },
    { key: "surname", labelKey: "users.fields.surname", required: true },
    { key: "status", labelKey: "users.fields.status" },
    { key: "locale", labelKey: "users.fields.locale" },
    { key: "role_slugs", labelKey: "users.fields.roles" },
  ],
  "platform.roles": [
    { key: "name", labelKey: "roles.fields.name", required: true },
    { key: "slug", labelKey: "roles.fields.slug", required: true },
    { key: "description", labelKey: "roles.fields.description" },
    { key: "permission_slugs", labelKey: "roles.fields.permissions" },
  ],
  "tenant.branches": [
    { key: "code", labelKey: "branches.fields.code", required: true },
    { key: "name", labelKey: "branches.fields.name", required: true },
    { key: "city", labelKey: "branches.fields.city" },
    { key: "district", labelKey: "branches.fields.district" },
    { key: "address", labelKey: "branches.fields.address" },
    { key: "phone", labelKey: "branches.fields.phone" },
    { key: "email", labelKey: "branches.fields.email" },
    { key: "latitude", labelKey: "branches.fields.latitude" },
    { key: "longitude", labelKey: "branches.fields.longitude" },
    { key: "prep_minutes", labelKey: "branches.fields.prep_minutes" },
    { key: "is_active", labelKey: "branches.fields.is_active" },
  ],
  "tenant.staff": [
    { key: "name", labelKey: "staff.fields.name", required: true },
    { key: "title", labelKey: "staff.fields.title" },
    { key: "phone", labelKey: "staff.fields.phone" },
    { key: "email", labelKey: "staff.fields.email" },
    { key: "branch_codes", labelKey: "staff.fields.branch_codes" },
    { key: "color", labelKey: "staff.fields.color" },
    { key: "show_online", labelKey: "staff.fields.show_online" },
    { key: "is_active", labelKey: "staff.fields.is_active" },
    { key: "uuid", labelKey: "staff.columns.uuid" },
  ],
  "tenant.resources": [
    {
      key: "branch_code",
      labelKey: "resources.fields.branch_code",
      required: true,
    },
    { key: "kind", labelKey: "resources.fields.kind", required: true },
    { key: "name", labelKey: "resources.fields.name", required: true },
    { key: "zone", labelKey: "resources.fields.zone" },
    { key: "min_capacity", labelKey: "resources.fields.min_capacity" },
    { key: "capacity", labelKey: "resources.fields.capacity" },
    { key: "combinable", labelKey: "resources.fields.combinable" },
    { key: "show_online", labelKey: "resources.fields.show_online" },
    { key: "is_active", labelKey: "resources.fields.is_active" },
    { key: "sort_order", labelKey: "resources.fields.sort_order" },
    { key: "description", labelKey: "resources.fields.description" },
  ],
  "tenant.customers": [
    {
      key: "first_name",
      labelKey: "customers.fields.first_name",
      required: true,
    },
    { key: "last_name", labelKey: "customers.fields.last_name" },
    { key: "phone", labelKey: "customers.fields.phone" },
    { key: "email", labelKey: "customers.fields.email" },
    { key: "birth_date", labelKey: "customers.fields.birth_date" },
    { key: "tags", labelKey: "customers.fields.tags" },
    { key: "sms_consent", labelKey: "customers.fields.sms_consent" },
    { key: "email_consent", labelKey: "customers.fields.email_consent" },
    { key: "notes", labelKey: "customers.fields.notes" },
  ],
  "tenant.products": [
    { key: "sku", labelKey: "products.fields.sku", required: true },
    { key: "name", labelKey: "products.fields.name", required: true },
    { key: "barcode", labelKey: "products.fields.barcode" },
    { key: "unit", labelKey: "products.fields.unit" },
    { key: "price", labelKey: "products.fields.price_short", required: true },
    { key: "cost", labelKey: "products.fields.cost_short" },
    {
      key: "low_stock_threshold",
      labelKey: "products.fields.low_stock_threshold",
    },
    { key: "branch_codes", labelKey: "products.fields.branch_codes" },
    { key: "show_online", labelKey: "products.fields.show_online" },
    { key: "is_active", labelKey: "products.fields.is_active" },
    { key: "description", labelKey: "products.fields.description" },
  ],
  "tenant.services": [
    { key: "category", labelKey: "services.fields.category" },
    { key: "name", labelKey: "services.fields.name", required: true },
    {
      key: "duration_minutes",
      labelKey: "services.fields.duration",
      required: true,
    },
    { key: "buffer_before_minutes", labelKey: "services.fields.buffer_before" },
    { key: "buffer_after_minutes", labelKey: "services.fields.buffer_after" },
    { key: "price", labelKey: "services.fields.price_short", required: true },
    { key: "payment_mode", labelKey: "services.fields.payment_mode" },
    { key: "staff_mode", labelKey: "services.fields.staff_mode" },
    { key: "resource_kinds", labelKey: "services.fields.resource_kinds" },
    { key: "branch_codes", labelKey: "services.fields.branch_codes" },
    { key: "show_online", labelKey: "services.fields.show_online" },
    { key: "is_active", labelKey: "services.fields.is_active" },
    { key: "sort_order", labelKey: "services.fields.sort_order" },
    { key: "description", labelKey: "services.fields.description" },
  ],
};
