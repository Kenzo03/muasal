import Link from "next/link";
import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe, serverApi } from "@/lib/server-api";

export default async function Home() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("home");
  const { data } = await (await serverApi()).GET("/projects");
  const projects = data?.items ?? [];
  return (
    <main className="mx-auto max-w-4xl p-8">
      <p className="text-neutral-600">{t("signedInAs", { name: me.name })}</p>
      <div className="mt-6 flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("projects")}</h1>
        {me.is_admin && (
          <Link className="rounded bg-neutral-900 px-4 py-2 text-sm text-white" href="/projects/new">
            {t("newProject")}
          </Link>
        )}
      </div>
      {projects.length === 0 ? (
        <p className="mt-4 text-neutral-600">{me.is_admin ? t("noProjectsAdmin") : t("noProjects")}</p>
      ) : (
        <ul className="mt-4 divide-y rounded-lg border bg-white">
          {projects.map((p) => (
            <li key={p.id}>
              <Link className="flex items-baseline gap-3 p-4 hover:bg-neutral-50" href={`/p/${p.key}/modules`}>
                <span className="font-mono text-sm text-neutral-500">{p.key}</span>
                <span>{p.name}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
