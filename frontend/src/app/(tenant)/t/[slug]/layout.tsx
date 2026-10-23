"use client";

import type { ReactNode } from "react";
import { useParams } from "next/navigation";

import { AppLayout } from "@/components/layout";
import { TenantProvider, TenantRouteGuard } from "@/features/tenant";

/** Organization shell — `/t/{slug}` */
export default function TenantLayout({ children }: { children: ReactNode }) {
  const params = useParams<{ slug: string }>();
  const slug = decodeURIComponent(params.slug ?? "");

  return (
    <TenantProvider key={slug} slug={slug}>
      <TenantRouteGuard>
        <AppLayout variant="tenant">{children}</AppLayout>
      </TenantRouteGuard>
    </TenantProvider>
  );
}
