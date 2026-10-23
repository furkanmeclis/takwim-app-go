"use client";

import type { ReactNode } from "react";

import { CustomerPanelShell } from "@/features/customer-panel/components/customer-panel-shell";

export default function CustomerAreaLayout({
  children,
}: {
  children: ReactNode;
}) {
  return <CustomerPanelShell>{children}</CustomerPanelShell>;
}
