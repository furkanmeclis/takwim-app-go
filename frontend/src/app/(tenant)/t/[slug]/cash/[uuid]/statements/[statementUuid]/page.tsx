import { StatementDetailPage } from "@/features/cash";

type PageProps = {
  params: Promise<{ slug: string; uuid: string; statementUuid: string }>;
};

export default async function TenantCashStatementPage({ params }: PageProps) {
  const { uuid, statementUuid } = await params;
  return (
    <StatementDetailPage accountUuid={uuid} statementUuid={statementUuid} />
  );
}
