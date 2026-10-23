import type { ReactNode } from "react";

import { PlatformSettingsLayout } from "@/features/settings";

/** System Settings shell — grouped sub-sidebar around every `/platform/settings/*` page. */
export default function Layout({ children }: { children: ReactNode }) {
  return <PlatformSettingsLayout>{children}</PlatformSettingsLayout>;
}
