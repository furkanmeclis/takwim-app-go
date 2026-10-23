import { BranchDetailPage } from "@/features/branches";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantBranchDetailPage({ params }: PageProps) {
  const { uuid } = await params;
  return <BranchDetailPage uuid={uuid} />;
}
