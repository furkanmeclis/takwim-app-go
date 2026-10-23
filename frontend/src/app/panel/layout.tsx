import type { Metadata } from "next";
import type { ReactNode } from "react";

import { i18nConfig } from "@/config/i18n";
import { translate } from "@/lib/i18n/messages";

// Personal pages: never indexed, no referrer to the businesses' sites.
export const metadata: Metadata = {
  title: translate(i18nConfig.defaultLocale, "customer_panel.title"),
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

/** Customer panel (takwim.link/panel): its own session, no staff shell. */
export default function CustomerPanelLayout({
  children,
}: {
  children: ReactNode;
}) {
  return children;
}
