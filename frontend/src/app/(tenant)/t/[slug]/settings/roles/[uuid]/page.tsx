import { TenantRoleDetailPage } from "@/features/tenant-roles";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantRoleDetailRoute({ params }: PageProps) {
  const { uuid } = await params;
  return <TenantRoleDetailPage uuid={uuid} />;
}
