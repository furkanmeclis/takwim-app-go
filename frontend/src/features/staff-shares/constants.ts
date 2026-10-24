import type {
  StaffPayoutKind,
  StaffShareBasis,
  StaffShareItemType,
  StaffShareLineKind,
  StaffShareStatus,
} from "@/features/staff-shares/types";

/** Engine / live slugs (activity, realtime). */
export const STAFF_SHARE_RULES_RESOURCE = "tenant.staff_share_rules" as const;
export const STAFF_SHARE_LINES_RESOURCE = "tenant.staff_share_lines" as const;
export const STAFF_SHARE_PAYOUTS_RESOURCE =
  "tenant.staff_share_payouts" as const;

/** Sector module that turns staff shares on. */
export const STAFF_SHARES_MODULE = "staff_shares" as const;

export const STAFF_SHARE_ITEM_TYPES: StaffShareItemType[] = [
  "service",
  "product",
];
export const STAFF_SHARE_BASES: StaffShareBasis[] = ["percent", "fixed"];
export const STAFF_SHARE_STATUSES: StaffShareStatus[] = ["pending", "payable"];
export const STAFF_SHARE_LINE_KINDS: StaffShareLineKind[] = [
  "share",
  "reversal",
];
export const STAFF_PAYOUT_KINDS: StaffPayoutKind[] = ["payout", "reversal"];

export const ITEM_TYPE_LABEL_KEYS: Record<StaffShareItemType, string> = {
  service: "staff_shares.item_types.service",
  product: "staff_shares.item_types.product",
};

export const BASIS_LABEL_KEYS: Record<StaffShareBasis, string> = {
  percent: "staff_shares.bases.percent",
  fixed: "staff_shares.bases.fixed",
};

export const STATUS_LABEL_KEYS: Record<StaffShareStatus, string> = {
  pending: "staff_shares.statuses.pending",
  payable: "staff_shares.statuses.payable",
};

export const LINE_KIND_LABEL_KEYS: Record<StaffShareLineKind, string> = {
  share: "staff_shares.line_kinds.share",
  reversal: "staff_shares.line_kinds.reversal",
};

export const PAYOUT_KIND_LABEL_KEYS: Record<StaffPayoutKind, string> = {
  payout: "staff_shares.payout_kinds.payout",
  reversal: "staff_shares.payout_kinds.reversal",
};

/** API limits (see `StaffShareRuleInput`, `StaffSharePayoutInput`). */
export const MAX_NOTE = 500;
