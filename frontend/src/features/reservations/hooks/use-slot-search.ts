"use client";

import { useMemo } from "react";

import { useAvailability } from "@/features/reservations/hooks/use-reservations";
import { toLineInputs } from "@/features/reservations/schemas/reservation-forms";
import type { AvailabilitySearchRequest } from "@/features/reservations/types";

/**
 * Free starts of one day for the lines of a form (panel channel, 15 minute
 * steps). `ready` is false until branch, every service and the day are set,
 * and, when `partyRequired` (restaurant flow), the number of guests.
 */
export function useSlotSearch({
  branchUuid,
  lines,
  date,
  partySize,
  zone,
  partyRequired = false,
}: {
  branchUuid: string;
  lines: { service_uuid: string; staff_uuid: string }[];
  date: string;
  /** Valid guests or undefined. */
  partySize?: number;
  /** Area of the branch or undefined for any. */
  zone?: string;
  partyRequired?: boolean;
}) {
  // Form arrays are new on every render; key the request on their content.
  const linesKey = JSON.stringify(toLineInputs(lines));
  const ready =
    Boolean(branchUuid) &&
    Boolean(date) &&
    lines.length > 0 &&
    lines.every((line) => line.service_uuid) &&
    (!partyRequired || partySize !== undefined);
  const body = useMemo<AvailabilitySearchRequest | null>(
    () =>
      ready
        ? {
            branch_uuid: branchUuid,
            lines: JSON.parse(linesKey) as AvailabilitySearchRequest["lines"],
            from: date,
            to: date,
            step_minutes: 15,
            channel: "panel",
            ...(partySize !== undefined ? { party_size: partySize } : {}),
            ...(zone ? { zone } : {}),
          }
        : null,
    [branchUuid, date, linesKey, partySize, ready, zone],
  );
  const query = useAvailability(body);
  return { query, ready };
}
