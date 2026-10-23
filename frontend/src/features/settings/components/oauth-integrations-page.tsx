"use client";

import { useState } from "react";
import type { LucideIcon } from "lucide-react";

import { appleNavIcon } from "@/components/icons/apple-icon";
import { facebookNavIcon } from "@/components/icons/facebook-icon";
import { githubNavIcon } from "@/components/icons/github-icon";
import { googleNavIcon } from "@/components/icons/google-icon";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { GitHubIntegrationSettingsPage } from "@/features/integrations/github";
import { OAuthProviderSettingsPage } from "@/features/integrations/oauth/components/oauth-provider-settings-page";
import { SettingsForbidden } from "@/features/settings/components/settings-forbidden";
import { SettingsPageHeader } from "@/features/settings/components/settings-page-header";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type ProviderTab = {
  id: "google" | "github" | "facebook" | "apple";
  labelKey: string;
  icon: LucideIcon;
  read: string;
};

const PROVIDER_TABS: ProviderTab[] = [
  {
    id: "google",
    labelKey: "integrations.providers.google",
    icon: googleNavIcon,
    read: permissions.integrations.google.read,
  },
  {
    id: "github",
    labelKey: "integrations.providers.github",
    icon: githubNavIcon,
    read: permissions.integrations.github.read,
  },
  {
    id: "facebook",
    labelKey: "integrations.providers.facebook",
    icon: facebookNavIcon,
    read: permissions.integrations.facebook.read,
  },
  {
    id: "apple",
    labelKey: "integrations.providers.apple",
    icon: appleNavIcon,
    read: permissions.integrations.apple.read,
  },
];

/** OAuth / social login providers as tabs; `?tab=<provider>` deep-links one. */
export function OAuthIntegrationsPage({ initialTab }: { initialTab?: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const tabs = PROVIDER_TABS.filter((tab) => can(tab.read));
  const [selected, setSelected] = useState<string | undefined>(initialTab);

  if (tabs.length === 0) return <SettingsForbidden />;

  const activeTab = tabs.some((tab) => tab.id === selected)
    ? (selected as string)
    : tabs[0]!.id;

  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.subnav.oauth_providers")}
        description={t("integrations.description")}
      />

      <Tabs value={activeTab} onValueChange={setSelected} className="w-full">
        <TabsList className="flex w-full max-w-xl flex-wrap">
          {tabs.map((tab) => {
            const Icon = tab.icon;
            return (
              <TabsTrigger
                key={tab.id}
                value={tab.id}
                className="flex flex-1 items-center gap-2 text-xs"
              >
                <Icon className="size-3.5" />
                {t(tab.labelKey)}
              </TabsTrigger>
            );
          })}
        </TabsList>

        {tabs.map((tab) => (
          <TabsContent key={tab.id} value={tab.id} className="mt-4">
            {tab.id === "github" ? (
              <GitHubIntegrationSettingsPage />
            ) : (
              <OAuthProviderSettingsPage
                provider={tab.id}
                writePermission={permissions.integrations[tab.id].write}
              />
            )}
          </TabsContent>
        ))}
      </Tabs>
    </div>
  );
}
