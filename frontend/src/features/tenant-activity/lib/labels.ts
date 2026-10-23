import { FIELD_LABEL_KEYS as SETTINGS_FIELD_LABEL_KEYS } from "@/features/tenant-settings/constants";
import type {
  ActivityChange,
  TenantActivityEvent,
} from "@/features/tenant-activity/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function labelOr(t: Translate, key: string, fallback: string) {
  const label = t(key);
  return label === key ? fallback : label;
}

/** `branches.updated` → "Şube güncellendi" (raw action when unknown). */
export function activityActionLabel(t: Translate, action: string) {
  return labelOr(t, `activity.actions.${action}`, action);
}

/** `tenant.branches` → "Şubeler" (raw resource when unknown). */
export function activityResourceLabel(t: Translate, resource: string) {
  return labelOr(t, `activity.resources.${resource}`, resource);
}

function fieldLabel(t: Translate, resource: string, path: string) {
  const own = `activity.fields.${resource}.${path}`;
  const label = t(own);
  if (label !== own) return label;
  if (resource === "tenant.settings" && SETTINGS_FIELD_LABEL_KEYS[path]) {
    return t(SETTINGS_FIELD_LABEL_KEYS[path]);
  }
  if (resource === "tenant.branches") {
    const key = `branches.fields.${path}`;
    const branch = t(key);
    if (branch !== key) return branch;
  }
  if (resource === "tenant.staff") {
    const key = `staff.fields.${path}`;
    const staff = t(key);
    if (staff !== key) return staff;
  }
  return null;
}

/**
 * Label of a changed field: `activity.fields.{resource}.{path}` first, then
 * the resource's own form labels (settings, branches). A nested path
 * (`working_hours.mon.open`) uses its longest labelled prefix and keeps the
 * rest as is; an unknown path is shown raw.
 */
export function activityFieldLabel(
  t: Translate,
  resource: string,
  path: string,
) {
  const parts = path.split(".");
  for (let n = parts.length; n > 0; n--) {
    const label = fieldLabel(t, resource, parts.slice(0, n).join("."));
    if (label) {
      const rest = parts.slice(n).join(".");
      return rest ? `${label} · ${rest}` : label;
    }
  }
  return path;
}

export function activityActorName(event: TenantActivityEvent) {
  if (!event.actor) return null;
  const full = `${event.actor.name} ${event.actor.surname}`.trim();
  return full || event.actor.email;
}

/** Before / after of the changed fields, sorted by path. */
export function activityChanges(event: TenantActivityEvent) {
  const raw = (event.payload as Record<string, unknown>).changes;
  if (!raw || typeof raw !== "object") return [];
  return Object.entries(raw as Record<string, ActivityChange>)
    .map(([path, change]) => ({ path, ...change }))
    .sort((a, b) => a.path.localeCompare(b.path));
}

/** Payload fields other than `changes` (e.g. a service point's name). */
export function activityDetails(event: TenantActivityEvent) {
  return Object.entries(event.payload as Record<string, unknown>)
    .filter(([key]) => key !== "changes")
    .map(([key, value]) => ({ key, value }));
}

/** A stored value as text: "—" for empty, yes / no, JSON for objects. */
export function formatActivityValue(t: Translate, value: unknown) {
  if (value === null || value === undefined || value === "") return "—";
  if (typeof value === "boolean")
    return value ? t("table.true") : t("table.false");
  if (typeof value === "object") {
    if (Array.isArray(value) && value.length === 0) return "—";
    return JSON.stringify(value);
  }
  return String(value);
}
