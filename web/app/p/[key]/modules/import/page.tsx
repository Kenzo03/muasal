import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getProject } from "@/lib/server-api";
import TreeImport from "./TreeImport";

export default async function TreeImportPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project || project.role !== "admin") notFound();
  const t = await getTranslations("treeImport");
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <TreeImport projectKey={key} />
      </main>
    </>
  );
}
