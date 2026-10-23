import { TenantExportDownloadPage } from "@/features/io";

type PageProps = {
  params: Promise<{ slug: string; uuid: string }>;
};

/** Action link of an organization's "export ready" notification. */
export default async function TenantExportDownloadRoute({ params }: PageProps) {
  const { uuid } = await params;
  return <TenantExportDownloadPage uuid={uuid} />;
}
