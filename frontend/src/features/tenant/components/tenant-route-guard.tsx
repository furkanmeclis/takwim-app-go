"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Ban, CalendarX2, ShieldAlert, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useAuthStore } from "@/lib/auth/session-store";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/** API error codes the guard turns into dedicated cards. */
export const TENANT_ACCESS_CODES = {
  notMember: "NO_TENANT_MEMBERSHIP",
  suspended: "ORGANIZATION_SUSPENDED",
  expired: "ORGANIZATION_ACCESS_EXPIRED",
} as const;

type AccessIssue = "not_member" | "suspended" | "expired" | "unavailable";

function issueFor(code: string | undefined): AccessIssue {
  switch (code) {
    case TENANT_ACCESS_CODES.notMember:
      return "not_member";
    case TENANT_ACCESS_CODES.suspended:
      return "suspended";
    case TENANT_ACCESS_CODES.expired:
      return "expired";
    default:
      return "unavailable";
  }
}

const ISSUE_VIEW = {
  not_member: {
    icon: ShieldAlert,
    title: "tenant.access.not_member_title",
    body: "tenant.access.not_member_body",
  },
  suspended: {
    icon: Ban,
    title: "tenant.access.suspended_title",
    body: "tenant.access.suspended_body",
  },
  expired: {
    icon: CalendarX2,
    title: "tenant.access.expired_title",
    body: "tenant.access.expired_body",
  },
  unavailable: {
    icon: TriangleAlert,
    title: "tenant.access.unavailable_title",
    body: "tenant.access.unavailable_body",
  },
} as const;

function PageLoader({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
      {label}
    </div>
  );
}

function TenantAccessCard({
  issue,
  slug,
  onRetry,
}: {
  issue: AccessIssue;
  slug: string;
  onRetry: () => void;
}) {
  const { t } = useLocale();
  const { user } = useAuth();
  const view = ISSUE_VIEW[issue];
  const Icon = view.icon;
  const home = user ? defaultHomeForUser(user) : routes.guest.login;

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md" data-tenant-access={issue}>
        <CardHeader>
          <div className="bg-muted text-muted-foreground mb-2 flex size-10 items-center justify-center rounded-full">
            <Icon className="size-5" />
          </div>
          <CardTitle>{t(view.title)}</CardTitle>
          <CardDescription>{t(view.body, { slug })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {user?.email ? (
            <p className="text-muted-foreground text-sm">
              {t("tenant.access.signed_in_as", { email: user.email })}
            </p>
          ) : null}
          {issue === "unavailable" ? (
            <Button variant="outline" onClick={onRetry}>
              {t("tenant.access.retry")}
            </Button>
          ) : null}
          <Button asChild>
            <Link href={home}>{t("tenant.access.go_home")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * Gate of the `/t/{slug}` shell: sign-in, then membership, suspension and
 * access window of the organization. Renders right away from the cached
 * `/me` membership when it is usable; the API answer replaces it.
 */
export function TenantRouteGuard({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { t } = useLocale();
  const { bootstrapped, isAuthenticated } = useAuth();
  const cachedUser = useAuthStore((s) => s.user);
  const tenant = useTenant();

  useEffect(() => {
    if (!bootstrapped || isAuthenticated) return;
    const next = encodeURIComponent(
      pathname || routes.tenant.home(tenant.slug),
    );
    router.replace(`${routes.guest.login}?next=${next}`);
  }, [bootstrapped, isAuthenticated, pathname, router, tenant.slug]);

  if (!bootstrapped && !cachedUser) {
    return <PageLoader label={t("common.loading")} />;
  }

  if (!isAuthenticated) return null;

  if (tenant.failed) {
    return (
      <TenantAccessCard
        issue={issueFor(tenant.error?.code)}
        slug={tenant.slug}
        onRetry={tenant.refetch}
      />
    );
  }

  if (!tenant.verified && !tenant.optimistic) {
    return <PageLoader label={t("common.loading")} />;
  }

  return <>{children}</>;
}
