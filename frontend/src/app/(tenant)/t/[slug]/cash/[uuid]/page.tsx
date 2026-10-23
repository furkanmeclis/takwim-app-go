import { CashAccountDetailPage } from "@/features/cash";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantCashAccountDetailPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <CashAccountDetailPage uuid={uuid} />;
}
