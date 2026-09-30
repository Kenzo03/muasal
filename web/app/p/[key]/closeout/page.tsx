import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getProject, serverApi } from "@/lib/server-api";
import CloseOut from "./CloseOut";

// Closing many tickets at once, each with its own decision record (MSL-65):
// the ticket list's bulk bar sends the ticked keys here.
export default async function CloseOutPage({ params, searchParams }: {
  params: Promise<{ key: string }>;
  searchParams: Promise<{ keys?: string }>;
}) {
  const { key } = await params;
  const { keys } = await searchParams;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("closeout");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [statuses, nodes] = await Promise.all([api.GET("/projects/{key}/statuses", path), api.GET("/projects/{key}/nodes", path)]);
  const picked = [...new Set((keys ?? "").split(",").map((k) => k.trim().toUpperCase()).filter((k) => k.startsWith(`${key}-`)))].slice(0, 100);
  return (
    <>
      <PageBar>
        <h1>{t("heading")}</h1>
        <span className="text-[13px] text-muted">{project.name}</span>
      </PageBar>
      <main className="flex flex-col gap-3 px-4 py-4 md:px-5">
        <p className="max-w-3xl text-[13px] text-muted">{t("intro")}</p>
        {project.role === "viewer" ? (
          <p className="text-muted">{t("viewers")}</p>
        ) : (
          <CloseOut keys={picked} statuses={statuses.data?.items ?? []} nodes={nodes.data?.items ?? []} />
        )}
      </main>
    </>
  );
}
