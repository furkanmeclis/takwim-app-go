import { StaffEditPage } from "@/features/staff";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantStaffEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <StaffEditPage uuid={uuid} />;
}
