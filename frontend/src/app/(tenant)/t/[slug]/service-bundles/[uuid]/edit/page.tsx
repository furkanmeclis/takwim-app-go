import { BundleEditPage } from "@/features/service-bundles";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

export default async function TenantServiceBundleEditPage({
  params,
}: PageProps) {
  const { uuid } = await params;
  return <BundleEditPage uuid={uuid} />;
}
