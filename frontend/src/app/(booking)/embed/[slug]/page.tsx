import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { EmbedBookingPage } from "@/features/business-page";
import {
  parseEmbedContext,
  parsePreselect,
} from "@/features/business-page/lib/embed";
import { getPublicBusiness } from "@/features/business-page/server/get-business";

type Props = {
  params: Promise<{ slug: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const lookup = await getPublicBusiness(slug);
  return {
    title: lookup.status === "found" ? lookup.business.name : undefined,
    // The business page is the indexed one; the frame is not.
    robots: { index: false, follow: false },
  };
}

/**
 * Embedded booking screen (TEC-908): takwim.link/embed/{slug}, framed by the
 * loader script (`/embed.js`) on the business's own site. Only the sites in
 * the business's embed settings may frame it (`frame-ancestors`, set by
 * `src/proxy.ts`). The pre-selection is kept only when well formed; the
 * wizard applies it when the API lists it.
 */
export default async function Page({ params, searchParams }: Props) {
  const [{ slug }, query] = await Promise.all([params, searchParams]);
  const lookup = await getPublicBusiness(slug);
  if (lookup.status === "not_found") notFound();
  return (
    <EmbedBookingPage
      slug={slug}
      ctx={parseEmbedContext(query)}
      preselect={parsePreselect(query)}
      initialBusiness={lookup.status === "found" ? lookup.business : undefined}
    />
  );
}
