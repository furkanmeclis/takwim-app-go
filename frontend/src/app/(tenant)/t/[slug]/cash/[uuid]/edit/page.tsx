import { CashAccountEditPage } from "@/features/cash";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantCashAccountEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <CashAccountEditPage uuid={uuid} />;
}
