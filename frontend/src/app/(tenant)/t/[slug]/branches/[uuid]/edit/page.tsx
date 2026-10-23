import { BranchEditPage } from "@/features/branches";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantBranchEditPage({ params }: PageProps) {
  const { uuid } = await params;
  return <BranchEditPage uuid={uuid} />;
}
