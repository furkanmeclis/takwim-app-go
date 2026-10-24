"use client";

import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";

import { publicBookingService } from "@/features/business-page/services/public-booking.service";
import type {
  PublicAvailabilityRequest,
  PublicBusiness,
  PublicClassesQuery,
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
  products: (slug: string, branch: string) => [
    ...publicBookingKeys.all(slug),
    "products",
    branch,
  ],
  availability: (slug: string, body: PublicAvailabilityRequest | null) => [
    ...publicBookingKeys.all(slug),
    "availability",
    body,
  ],
  classes: (slug: string, query: PublicClassesQuery | null) => [
    ...publicBookingKeys.all(slug),
    "classes",
    query,
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

/** Products of the branch for the optional product step (off = not asked). */
export function usePublicProducts(
  slug: string,
  branchUuid: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: publicBookingKeys.products(slug, branchUuid),
    queryFn: () => publicBookingService.products(slug, branchUuid),
    enabled: enabled && Boolean(branchUuid),
    staleTime: 30_000,
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

/** Class timetable of the branch (TEC-990; null query = not asked yet). */
export function usePublicClasses(
  slug: string,
  query: PublicClassesQuery | null,
) {
  return useQuery({
    queryKey: publicBookingKeys.classes(slug, query),
    queryFn: () => publicBookingService.classes(slug, query!),
    enabled: query !== null,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

/** Polls of a booking just back from the payment page (about two minutes). */
const PAYMENT_WATCH_POLLS = 40;

/**
 * Status view of a booking behind its access token. `watchPayment` (back
 * from the provider's page) looks every few seconds while the payment is
 * still open: only the provider's server notification records the result.
 * `watchUntil` (the embedded screen, whose payment runs in another tab)
 * does the same until that time.
 */
export function usePublicReservation(
  slug: string,
  uuid: string,
  accessToken: string | null,
  watchPayment = false,
  watchUntil: number | null = null,
) {
  return useQuery({
    queryKey: publicBookingKeys.reservation(slug, uuid),
    queryFn: () => publicBookingService.reservation(slug, uuid, accessToken!),
    enabled: Boolean(accessToken),
    // A request waiting for the business or for payment changes on the
    // business side; look again now and then.
    refetchInterval: (query) => {
      const data = query.state.data;
      if (
        watchPayment &&
        data?.payment_status === "online_pending" &&
        query.state.dataUpdateCount < PAYMENT_WATCH_POLLS
      ) {
        return 3_000;
      }
      if (
        watchUntil &&
        data?.payment_status === "online_pending" &&
        Date.now() < watchUntil
      ) {
        return 3_000;
      }
      return data?.status === "requested" ? 30_000 : false;
    },
  });
}

/** Opens the provider's page for the booking's online payment. */
export function useStartPublicPayment(
  slug: string,
  uuid: string,
  accessToken: string | null,
) {
  return useMutation({
    mutationFn: (locale: string) =>
      publicBookingService.startPayment(slug, uuid, accessToken!, locale),
  });
}
