"use client";

import type { ReactNode } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { AccountLayout } from "@/components/layout";

/** Pages for every signed-in user, outside the platform / tenant shells. */
export default function AccountGroupLayout({
  children,
}: {
  children: ReactNode;
}) {
  return (
    <RouteGuard mode="account">
      <AccountLayout>{children}</AccountLayout>
    </RouteGuard>
  );
}
