import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import NoteForm from "@/components/NoteForm";
import PageBar from "@/components/PageBar";
import { getProject, serverApi } from "@/lib/server-api";
import { panel } from "@/lib/ui";

export default async function NewNotePage({ params }: { params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project || project.role === "viewer") notFound();
  const t = await getTranslations("notes");
  const api = await serverApi();
  const path = { params: { path: { key } } };
  const [clients, nodes] = await Promise.all([api.GET("/projects/{key}/clients", path), api.GET("/projects/{key}/nodes", path)]);
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("newTitle")}</h1>
      </PageBar>
      <main className="px-4 py-4 md:px-5">
        <div className={panel}>
          <NoteForm projectKey={key} clients={clients.data?.items ?? []} nodes={nodes.data?.items ?? []} />
        </div>
      </main>
    </>
  );
}
