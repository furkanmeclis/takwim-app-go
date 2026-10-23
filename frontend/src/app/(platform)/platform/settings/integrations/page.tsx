import { OAuthIntegrationsPage } from "@/features/settings";

type PageProps = {
  searchParams: Promise<{ tab?: string | string[] }>;
};

export default async function Page({ searchParams }: PageProps) {
  const { tab } = await searchParams;
  return (
    <OAuthIntegrationsPage
      initialTab={typeof tab === "string" ? tab : undefined}
    />
  );
}
