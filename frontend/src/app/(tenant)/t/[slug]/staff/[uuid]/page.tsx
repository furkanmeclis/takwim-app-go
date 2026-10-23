import { StaffDetailPage } from "@/features/staff";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantStaffDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <StaffDetailPage uuid={uuid} />;
}
