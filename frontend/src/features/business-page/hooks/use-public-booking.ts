"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { publicBookingService } from "@/features/business-page/services/public-booking.service";
import type {
  PublicAvailabilityRequest,
  PublicBusiness,
} from "@/features/business-page/types";

export const publicBookingKeys = {
  all: (slug: string) => ["public-booking", slug] as const,
  business: (slug: string) => [...publicBookingKeys.all(slug), "business"],
  branches: (slug: string) => [...publicBookingKeys.all(slug), "branches"],
  services: (slug: string, branch: string) => [
    ...publicBookingKeys.all(slug),
    "services",
    branch,
  ],
  staff: (slug: string, branch: string) => [
    ...publicBookingKeys.all(slug),
    "staff",
    branch,
  ],
  availability: (slug: string, body: PublicAvailabilityRequest | null) => [
    ...publicBookingKeys.all(slug),
    "availability",
    body,
  ],
  reservation: (slug: string, uuid: string) => [
    ...publicBookingKeys.all(slug),
    "reservation",
    uuid,
  ],
};

/** Profile of the business; the server page passes the first answer. */
export function usePublicBusiness(slug: string, initial?: PublicBusiness) {
  return useQuery({
    queryKey: publicBookingKeys.business(slug),
    queryFn: () => publicBookingService.business(slug),
    initialData: initial,
    staleTime: 60_000,
  });
}

export function usePublicBranches(slug: string) {
  return useQuery({
    queryKey: publicBookingKeys.branches(slug),
    queryFn: () => publicBookingService.branches(slug),
    staleTime: 60_000,
  });
}

export function usePublicServices(slug: string, branchUuid: string) {
  return useQuery({
    queryKey: publicBookingKeys.services(slug, branchUuid),
    queryFn: () => publicBookingService.services(slug, branchUuid),
    enabled: Boolean(branchUuid),
    staleTime: 60_000,
  });
}

export function usePublicStaff(slug: string, branchUuid: string) {
  return useQuery({
    queryKey: publicBookingKeys.staff(slug, branchUuid),
    queryFn: () => publicBookingService.staff(slug, branchUuid),
    enabled: Boolean(branchUuid),
    staleTime: 60_000,
  });
}

/** Days and free starts of the online channel (null body = not asked yet). */
export function usePublicAvailability(
  slug: string,
  body: PublicAvailabilityRequest | null,
) {
  return useQuery({
    queryKey: publicBookingKeys.availability(slug, body),
    queryFn: () => publicBookingService.availability(slug, body!),
    enabled: body !== null,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

/** Status view of a booking behind its access token. */
export function usePublicReservation(
  slug: string,
  uuid: string,
  accessToken: string | null,
) {
  return useQuery({
    queryKey: publicBookingKeys.reservation(slug, uuid),
    queryFn: () => publicBookingService.reservation(slug, uuid, accessToken!),
    enabled: Boolean(accessToken),
    // A request waiting for the business or for payment changes on the
    // business side; look again now and then.
    refetchInterval: (query) =>
      query.state.data?.status === "requested" ? 30_000 : false,
  });
}
