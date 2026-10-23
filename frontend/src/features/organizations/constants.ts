import type {
  OrganizationAccessState,
  OrganizationStatus,
} from "@/features/organizations/types";

/** Engine slug: export, bulk and the live list `resource:platform.organizations`. */
export const ORGANIZATIONS_RESOURCE = "platform.organizations" as const;

export const ORGANIZATION_STATUSES = [
  "active",
  "suspended",
] as const satisfies readonly OrganizationStatus[];

export const ORGANIZATION_ACCESS_STATES = [
  "open",
  "upcoming",
  "expired",
] as const satisfies readonly OrganizationAccessState[];

export const ORGANIZATION_WINDOWS = ["unlimited", "limited"] as const;

export const STATUS_LABEL_KEYS: Record<OrganizationStatus, string> = {
  active: "organizations.status.active",
  suspended: "organizations.status.suspended",
};

export const STATUS_TONE: Record<OrganizationStatus, "success" | "danger"> = {
  active: "success",
  suspended: "danger",
};

export const ACCESS_LABEL_KEYS: Record<OrganizationAccessState, string> = {
  open: "organizations.access.open",
  upcoming: "organizations.access.upcoming",
  expired: "organizations.access.expired",
};

export const ACCESS_TONE: Record<
  OrganizationAccessState,
  "success" | "warning" | "danger"
> = {
  open: "success",
  upcoming: "warning",
  expired: "danger",
};

export const WINDOW_LABEL_KEYS: Record<
  (typeof ORGANIZATION_WINDOWS)[number],
  string
> = {
  unlimited: "organizations.window.unlimited",
  limited: "organizations.window.limited",
};
