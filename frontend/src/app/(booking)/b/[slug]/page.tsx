import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { i18nConfig } from "@/config/i18n";
import { BusinessPage } from "@/features/business-page";
import { getPublicBusiness } from "@/features/business-page/server/get-business";
import { translate } from "@/lib/i18n/messages";

type Props = { params: Promise<{ slug: string }> };

/** SEO: the business's name and sector; Turkish first like the page. */
export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const lookup = await getPublicBusiness(slug);
  if (lookup.status !== "found") {
    return { robots: { index: false, follow: false } };
  }
  const { business } = lookup;
  const locale = i18nConfig.defaultLocale;
  const description = translate(locale, "business_page.meta.description", {
    name: business.name,
    sector: translate(locale, `sectors.names.${business.sector}`),
  });
  return {
    title: business.name,
    description,
    alternates: { canonical: `/b/${encodeURIComponent(business.slug)}` },
    openGraph: {
      type: "website",
      title: business.name,
      description,
      locale: "tr_TR",
    },
    robots: { index: true, follow: true },
  };
}

/**
 * Business page: takwim.link/{slug} is served from /b/{slug}
 * (docs/design/business-page.md). An unknown, suspended or expired
 * business is a real 404.
 */
export default async function Page({ params }: Props) {
  const { slug } = await params;
  const lookup = await getPublicBusiness(slug);
  if (lookup.status === "not_found") notFound();
  return (
    <BusinessPage
      slug={slug}
      initialBusiness={lookup.status === "found" ? lookup.business : undefined}
    />
  );
}
