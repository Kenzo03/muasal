import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getProject, serverApi } from "@/lib/server-api";
import Builder from "./Builder";

export default async function NewSummaryPage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project || project.role === "viewer") notFound();
  const t = await getTranslations("summaries");
  const locale = await getLocale();
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes] = await Promise.all([api.GET("/projects/{key}/clients", path), api.GET("/projects/{key}/nodes", path)]);
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("newTitle")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <Builder projectKey={key} clients={clients.data?.items ?? []} nodes={nodes.data?.items ?? []} locale={locale === "en" ? "en" : "id"} />
      </main>
    </>
  );
}
