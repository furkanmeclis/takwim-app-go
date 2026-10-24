"use client";

import { CalendarX2 } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useBookingSurface } from "@/features/business-page/components/booking-surface";
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
import type { Preselect } from "@/features/business-page/lib/embed";
import { bookingOpen } from "@/features/business-page/lib/wizard";
import type {
  PublicBranch,
  PublicBusiness,
} from "@/features/business-page/types";
import { isApiError } from "@/lib/api";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";

/**
 * Cover photo: the theme's cover (TEC-966), else the first photo of the
 * first branch that has one.
 */
export function coverOf(business: PublicBusiness, branches: PublicBranch[]) {
  const own = business.theme?.cover_url;
  if (own) return resolveLogoSrc(assertServiceMediaURL(own));
  for (const branch of branches) {
    const url = branch.images[0]?.url;
    if (url) return resolveLogoSrc(assertServiceMediaURL(url));
  }
  return undefined;
}

/**
 * Hero of the page: cover, logo, name, sector and cities, then the theme's
 * welcome headline and text when the business wrote them.
 */
export function Hero({
  business,
  branches,
  cover,
}: {
  business: PublicBusiness;
  branches: PublicBranch[];
  /** Overrides the resolved cover (the panel preview). */
  cover?: string;
}) {
  const { t } = useLocale();
  const src = cover ?? coverOf(business, branches);
  const cities = [...new Set(branches.map((b) => b.city).filter(Boolean))];
  const headline = business.theme?.headline?.trim();
  const description = business.theme?.description?.trim();
  return (
    <section className="bg-card overflow-hidden rounded-xl border">
      {src ? (
        // API-owned image stream; next/image would need the API host.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={src} alt="" className="aspect-[3/1] w-full object-cover" />
      ) : (
        <div className="from-primary/15 via-muted to-background aspect-[5/1] w-full bg-gradient-to-br" />
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
      {headline || description ? (
        <div className="space-y-1 border-t px-4 py-4">
          {headline ? (
            <p className="font-display text-primary text-lg font-semibold">
              {headline}
            </p>
          ) : null}
          {description ? (
            <p className="text-muted-foreground text-sm whitespace-pre-line">
              {description}
            </p>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

/**
 * The booking part: the wizard, or why it is closed. The business page uses
 * channel `web`, the embedded screen `widget`.
 */
function BookingPanel({
  slug,
  business,
  preselect,
}: {
  slug: string;
  business: PublicBusiness;
  preselect?: Preselect;
}) {
  const { t } = useLocale();
  const { channel } = useBookingSurface();
  const branchesQuery = usePublicBranches(slug);
  const branches = branchesQuery.data ?? [];

  if (!bookingOpen(business, channel)) {
    return (
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
    );
  }
  if (branchesQuery.isLoading) {
    return <Skeleton className="h-64 w-full rounded-xl" />;
  }
  if (branchesQuery.isError) {
    return (
      <ErrorState
        title={t("business_page.load_failed")}
        onRetry={() => void branchesQuery.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (branches.length === 0) {
    return (
      <EmptyState
        title={t("business_page.no_branches_title")}
        description={t("business_page.no_branches_description")}
      />
    );
  }
  return (
    <BookingWizard
      slug={slug}
      business={business}
      branches={branches}
      preselect={preselect}
    />
  );
}

/**
 * The business page (takwim.link/{slug}): profile, branches and the booking
 * wizard. The profile comes from the server render; branches and the
 * catalog load in the browser through the same-origin BFF. The embedded
 * screen (TEC-908) shows the same page in its full view and only the
 * booking part in its compact view.
 */
export function BusinessPage({
  slug,
  initialBusiness,
  preselect,
}: {
  slug: string;
  initialBusiness?: PublicBusiness;
  preselect?: Preselect;
}) {
  const { t } = useLocale();
  const { embed } = useBookingSurface();
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

  if (embed?.view === "compact") {
    return (
      <BusinessPageShell slug={slug} business={business}>
        <BookingPanel slug={slug} business={business} preselect={preselect} />
      </BusinessPageShell>
    );
  }

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
          <BookingPanel slug={slug} business={business} preselect={preselect} />
        </TabsContent>
        <TabsContent value="info">
          <BusinessInfo business={business} branches={branches} />
        </TabsContent>
      </Tabs>
    </BusinessPageShell>
  );
}
