import { OrganizationDetailPage } from "@/features/organizations";

export default async function PlatformOrganizationDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ uuid: string }>;
  searchParams: Promise<{ tab?: string | string[] }>;
}) {
  const { uuid } = await params;
  const { tab } = await searchParams;
  const initialTab =
    typeof tab === "string" && /^[a-z_-]{1,32}$/.test(tab) ? tab : undefined;
  return <OrganizationDetailPage uuid={uuid} initialTab={initialTab} />;
}
