import type {
  OrganizationAccessState,
  OrganizationStatus,
  OverviewChannel,
  OverviewPayment,
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

type Tone = "default" | "success" | "warning" | "danger";

/** Channel set-up state on the 360 page (TEC-927). */
export const CHANNEL_STATUS_LABEL_KEYS: Record<
  OverviewChannel["status"],
  string
> = {
  ready: "organizations.setup.channel_status.ready",
  not_ready: "organizations.setup.channel_status.not_ready",
  unavailable: "organizations.setup.channel_status.unavailable",
};

export const CHANNEL_STATUS_TONES: Record<OverviewChannel["status"], Tone> = {
  ready: "success",
  not_ready: "warning",
  unavailable: "default",
};

export const PAYMENT_STATUS_LABEL_KEYS: Record<
  OverviewPayment["status"],
  string
> = {
  ready: "organizations.setup.payment_status.ready",
  not_connected: "organizations.setup.payment_status.not_connected",
  no_cash_account: "organizations.setup.payment_status.no_cash_account",
  provider_unavailable:
    "organizations.setup.payment_status.provider_unavailable",
};

export const PAYMENT_STATUS_TONES: Record<OverviewPayment["status"], Tone> = {
  ready: "success",
  not_connected: "warning",
  no_cash_account: "warning",
  provider_unavailable: "danger",
};
