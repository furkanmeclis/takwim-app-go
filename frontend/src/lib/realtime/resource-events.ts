import { RealtimeEvents } from "@/lib/realtime/events";
import type { RealtimeMessage } from "@/lib/realtime/types";

export type ResourceChangeAction = "created" | "updated" | "deleted" | "bulk";

/**
 * Payload of `resource.changed` publications (Go `resourceevents.Event`).
 * Published on `resource:{name}` (or a host-scoped channel) after list
 * mutations and once per bulk / import job.
 */
export type ResourceChangedEvent = {
  resource: string;
  action: ResourceChangeAction;
  uuids: string[];
  /** More rows changed than `uuids` lists: treat every row as changed. */
  truncated: boolean;
  at?: string;
};

const ACTIONS: ReadonlySet<string> = new Set([
  "created",
  "updated",
  "deleted",
  "bulk",
]);

/** Returns the resource-change payload of a realtime message, if it is one. */
export function parseResourceChanged(
  message: RealtimeMessage,
): ResourceChangedEvent | null {
  if (message.type !== RealtimeEvents.ResourceChanged) return null;
  const data = message.data as Record<string, unknown>;
  const resource = data.resource;
  const action = data.action;
  if (typeof resource !== "string" || typeof action !== "string") return null;
  if (!ACTIONS.has(action)) return null;
  const uuids = Array.isArray(data.uuids)
    ? data.uuids.filter((id): id is string => typeof id === "string")
    : [];
  return {
    resource,
    action: action as ResourceChangeAction,
    uuids,
    truncated: data.truncated === true,
    at: typeof data.at === "string" ? data.at : undefined,
  };
}
