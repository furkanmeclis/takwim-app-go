import { CustomerPackageDetailPage } from "@/features/session-packages";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantCustomerPackagePage({ params }: PageProps) {
  const { uuid } = await params;
  return <CustomerPackageDetailPage uuid={uuid} />;
}
