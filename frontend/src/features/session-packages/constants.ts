import type {
  CustomerPackageState,
  PackageMethod,
  PackageSessionStatus,
} from "@/features/session-packages/types";

/** Engine / live slugs (activity, realtime). */
export const SESSION_PACKAGES_RESOURCE = "tenant.session_packages" as const;
export const CUSTOMER_PACKAGES_RESOURCE = "tenant.customer_packages" as const;

/** Sector module that turns packages on. */
export const PACKAGES_MODULE = "packages" as const;

export const CUSTOMER_PACKAGE_STATES: CustomerPackageState[] = [
  "active",
  "used_up",
  "expired",
  "refunded",
];

export const STATE_LABEL_KEYS: Record<CustomerPackageState, string> = {
  active: "session_packages.states.active",
  used_up: "session_packages.states.used_up",
  expired: "session_packages.states.expired",
  refunded: "session_packages.states.refunded",
};

export const STATE_TONES = {
  active: "success",
  used_up: "default",
  expired: "warning",
  refunded: "danger",
} as const satisfies Record<
  CustomerPackageState,
  "default" | "success" | "warning" | "danger"
>;

export const SESSION_STATUS_LABEL_KEYS: Record<PackageSessionStatus, string> = {
  held: "session_packages.session_status.held",
  consumed: "session_packages.session_status.consumed",
  released: "session_packages.session_status.released",
};

export const SESSION_STATUS_TONES = {
  held: "warning",
  consumed: "success",
  released: "default",
} as const satisfies Record<
  PackageSessionStatus,
  "default" | "success" | "warning" | "danger"
>;

/** Why a session went back to its package. */
export const RELEASE_REASON_LABEL_KEYS: Record<string, string> = {
  rejected: "session_packages.release_reasons.rejected",
  cancelled: "session_packages.release_reasons.cancelled",
  no_show: "session_packages.release_reasons.no_show",
  expired: "session_packages.release_reasons.expired",
  rescheduled: "session_packages.release_reasons.rescheduled",
  removed: "session_packages.release_reasons.removed",
};

/** How a package is paid or refunded at the desk. */
export const PACKAGE_METHODS: PackageMethod[] = ["cash", "card", "transfer"];

export const METHOD_LABEL_KEYS: Record<string, string> = {
  cash: "session_packages.methods.cash",
  card: "session_packages.methods.card",
  transfer: "session_packages.methods.transfer",
  online: "session_packages.methods.online",
  other: "session_packages.methods.other",
};

/** API limits (see `SessionPackageInput`, `CustomerPackageSellRequest`). */
export const MAX_NAME = 160;
export const MAX_DESCRIPTION = 2000;
export const MAX_SESSIONS = 1000;
export const MAX_VALIDITY_DAYS = 3650;
export const MAX_NOTE = 500;
