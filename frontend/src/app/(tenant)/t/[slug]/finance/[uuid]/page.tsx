import { FinanceEntryDetailPage } from "@/features/finance";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantFinanceEntryDetailPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <FinanceEntryDetailPage uuid={uuid} />;
}
