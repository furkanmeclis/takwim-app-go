"use client";

import { EmbedFrame } from "@/features/business-page/components/booking-surface";
import { BusinessPage } from "@/features/business-page/components/business-page";
import { ReservationStatusPage } from "@/features/business-page/components/reservation-status-page";
import { usePublicBusiness } from "@/features/business-page/hooks/use-public-booking";
import type {
  EmbedContext,
  Preselect,
} from "@/features/business-page/lib/embed";
import type { PublicBusiness } from "@/features/business-page/types";

/** Allowed sites of the business (the targets of the frame's messages). */
function useAllowedOrigins(slug: string, initial?: PublicBusiness) {
  const query = usePublicBusiness(slug, initial);
  return query.data?.embed.allowed_origins ?? [];
}

/**
 * The embedded booking screen (`/embed/{slug}`, TEC-908): the business
 * page's wizard on channel `widget`, compact or full.
 */
export function EmbedBookingPage({
  slug,
  ctx,
  preselect,
  initialBusiness,
}: {
  slug: string;
  ctx: EmbedContext;
  preselect: Preselect;
  initialBusiness?: PublicBusiness;
}) {
  const allowed = useAllowedOrigins(slug, initialBusiness);
  return (
    <EmbedFrame ctx={ctx} allowedOrigins={allowed}>
      <BusinessPage
        slug={slug}
        initialBusiness={initialBusiness}
        preselect={preselect}
      />
    </EmbedFrame>
  );
}

/** Status of a booking made in the embedded screen. */
export function EmbedReservationPage({
  slug,
  uuid,
  ctx,
  initialBusiness,
}: {
  slug: string;
  uuid: string;
  ctx: EmbedContext;
  initialBusiness?: PublicBusiness;
}) {
  const allowed = useAllowedOrigins(slug, initialBusiness);
  return (
    <EmbedFrame ctx={ctx} allowedOrigins={allowed}>
      <ReservationStatusPage
        slug={slug}
        uuid={uuid}
        initialBusiness={initialBusiness}
      />
    </EmbedFrame>
  );
}
