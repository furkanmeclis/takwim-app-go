import { CustomerPackageDetailPage } from "@/features/customer-panel/components/customer-package-detail-page";

export default async function CustomerPackageRoute({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = await params;
  return <CustomerPackageDetailPage uuid={uuid} />;
}
