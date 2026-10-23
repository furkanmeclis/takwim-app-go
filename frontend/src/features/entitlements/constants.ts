import type {
  Addon,
  PlanCode,
  UsageMetric,
} from "@/features/entitlements/types";

export const PLAN_LABEL_KEYS: Record<PlanCode, string> = {
  starter: "entitlements.plans.starter",
  growth: "entitlements.plans.growth",
  enterprise: "entitlements.plans.enterprise",
};

export const ADDON_LABEL_KEYS: Record<Addon, string> = {
  confirmation: "entitlements.addons.confirmation",
};

export const ADDON_HINT_KEYS: Record<Addon, string> = {
  confirmation: "entitlements.addons.confirmation_hint",
};

export const USAGE_METRIC_LABEL_KEYS: Record<UsageMetric, string> = {
  sms: "entitlements.metrics.sms",
  whatsapp: "entitlements.metrics.whatsapp",
  email: "entitlements.metrics.email",
  voice_call: "entitlements.metrics.voice_call",
};
