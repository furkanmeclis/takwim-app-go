import { OrganizationEditPage } from "@/features/organizations";

export default async function PlatformOrganizationEditPage({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <OrganizationEditPage uuid={uuid} />;
}
