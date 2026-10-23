import { CustomerDetailPage } from "@/features/customers";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantCustomerDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <CustomerDetailPage uuid={uuid} />;
}
