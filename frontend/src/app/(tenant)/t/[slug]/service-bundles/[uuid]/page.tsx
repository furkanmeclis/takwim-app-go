import { BundleDetailPage } from "@/features/service-bundles";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantServiceBundleDetailPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <BundleDetailPage uuid={uuid} />;
}
