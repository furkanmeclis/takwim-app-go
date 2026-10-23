import type { Metadata } from "next";

import { ReservationStatusPage } from "@/features/business-page";
import { getPublicBusiness } from "@/features/business-page/server/get-business";

// A booking's status: personal data behind the access token of the link's
// fragment; keep it out of indexes and referrers.
export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function Page({
  params,
}: {
  params: Promise<{ slug: string; uuid: string }>;
}) {
  const { slug, uuid } = await params;
  const lookup = await getPublicBusiness(slug);
  return (
    <ReservationStatusPage
      slug={slug}
      uuid={uuid}
      initialBusiness={lookup.status === "found" ? lookup.business : undefined}
    />
  );
}
