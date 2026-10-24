import type { Metadata } from "next";

import { EmbedReservationPage } from "@/features/business-page";
import { parseEmbedContext } from "@/features/business-page/lib/embed";
import { getPublicBusiness } from "@/features/business-page/server/get-business";

// A booking's status: personal data behind the access token of the link's
// fragment; keep it out of indexes and referrers.
export const metadata: Metadata = {
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ slug: string; uuid: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const [{ slug, uuid }, query] = await Promise.all([params, searchParams]);
  const lookup = await getPublicBusiness(slug);
  return (
    <EmbedReservationPage
      slug={slug}
      uuid={uuid}
      ctx={parseEmbedContext(query)}
      initialBusiness={lookup.status === "found" ? lookup.business : undefined}
    />
  );
}
