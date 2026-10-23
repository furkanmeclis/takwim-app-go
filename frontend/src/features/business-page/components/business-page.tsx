"use client";

import { CalendarX2 } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BookingWizard } from "@/features/business-page/components/booking-wizard";
import { BusinessInfo } from "@/features/business-page/components/business-info";
import {
  BusinessAvatar,
  BusinessPageShell,
} from "@/features/business-page/components/business-page-shell";
import {
  usePublicBranches,
  usePublicBusiness,
} from "@/features/business-page/hooks/use-public-booking";
import { bookingOpen } from "@/features/business-page/lib/wizard";
import type {
  PublicBranch,
  PublicBusiness,
} from "@/features/business-page/types";
import { isApiError } from "@/lib/api";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";

/** Cover photo: the first photo of the first branch that has one. */
function coverOf(branches: PublicBranch[]) {
  for (const branch of branches) {
    const url = branch.images[0]?.url;
    if (url) return resolveLogoSrc(assertServiceMediaURL(url));
  }
  return undefined;
}

function Hero({
  business,
  branches,
}: {
  business: PublicBusiness;
  branches: PublicBranch[];
}) {
  const { t } = useLocale();
  const cover = coverOf(branches);
  const cities = [...new Set(branches.map((b) => b.city).filter(Boolean))];
  return (
    <section className="bg-card overflow-hidden rounded-xl border">
      {cover ? (
        // API-owned image stream; next/image would need the API host.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={cover} alt="" className="aspect-[3/1] w-full object-cover" />
      ) : (
        <div className="from-muted to-background aspect-[5/1] w-full bg-gradient-to-br" />
      )}
      <div className="flex items-end gap-3 px-4 pb-4">
        <BusinessAvatar
          business={business}
          className="border-background -mt-8 size-16 border-4 text-lg"
        />
        <div className="min-w-0 pt-2">
          <h1 className="truncate text-xl font-semibold">{business.name}</h1>
          <p className="text-muted-foreground truncate text-sm">
            {[t(`sectors.names.${business.sector}`), ...cities].join(" · ")}
          </p>
        </div>
      </div>
    </section>
  );
}

/**
 * The business page (takwim.link/{slug}): profile, branches and the booking
 * wizard. The profile comes from the server render; branches and the
 * catalog load in the browser through the same-origin BFF.
 */
export function BusinessPage({
  slug,
  initialBusiness,
}: {
  slug: string;
  initialBusiness?: PublicBusiness;
}) {
  const { t } = useLocale();
  const businessQuery = usePublicBusiness(slug, initialBusiness);
  const branchesQuery = usePublicBranches(slug);
  const business = businessQuery.data;
  const branches = branchesQuery.data ?? [];

  if (!business) {
    const notFound =
      isApiError(businessQuery.error) && businessQuery.error.status === 404;
    return (
      <BusinessPageShell slug={slug}>
        {businessQuery.isError ? (
          <ErrorState
            title={
              notFound
                ? t("business_page.not_found_title")
                : t("business_page.load_failed")
            }
            onRetry={notFound ? undefined : () => void businessQuery.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : (
          <Skeleton className="h-48 w-full rounded-xl" />
        )}
      </BusinessPageShell>
    );
  }

  const open = bookingOpen(business);

  return (
    <BusinessPageShell slug={slug} business={business}>
      <Hero business={business} branches={branches} />
      <Tabs defaultValue="book" className="gap-4">
        <TabsList className="w-full">
          <TabsTrigger value="book">
            {t(
              `business_page.tabs.book_${business.terms.booking ?? "appointment"}`,
            )}
          </TabsTrigger>
          <TabsTrigger value="info">{t("business_page.tabs.info")}</TabsTrigger>
        </TabsList>
        <TabsContent value="book" className="space-y-4">
          {!open ? (
            <Alert>
              <CalendarX2 />
              <AlertTitle>{t("business_page.closed_title")}</AlertTitle>
              <AlertDescription>
                {business.contact.phone
                  ? t("business_page.closed_call", {
                      phone: business.contact.phone,
                    })
                  : t("business_page.closed_description")}
              </AlertDescription>
            </Alert>
          ) : branchesQuery.isLoading ? (
            <Skeleton className="h-64 w-full rounded-xl" />
          ) : branchesQuery.isError ? (
            <ErrorState
              title={t("business_page.load_failed")}
              onRetry={() => void branchesQuery.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : branches.length === 0 ? (
            <EmptyState
              title={t("business_page.no_branches_title")}
              description={t("business_page.no_branches_description")}
            />
          ) : (
            <BookingWizard
              slug={slug}
              business={business}
              branches={branches}
            />
          )}
        </TabsContent>
        <TabsContent value="info">
          <BusinessInfo business={business} branches={branches} />
        </TabsContent>
      </Tabs>
    </BusinessPageShell>
  );
}
