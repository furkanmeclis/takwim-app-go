import { CustomerEditPage } from "@/features/customers";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantCustomerEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <CustomerEditPage uuid={uuid} />;
}
