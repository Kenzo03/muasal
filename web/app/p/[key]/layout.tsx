import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getProject } from "@/lib/server-api";

// Shared by every project page. Projects the user does not belong to answer 404.
export default async function ProjectLayout({ children, params }: { children: React.ReactNode; params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  if (!project) notFound();
  const t = await getTranslations("project");
  return (
    <div className="mx-auto max-w-6xl p-6">
      <div className="flex items-baseline gap-3">
        <span className="font-mono text-sm text-neutral-500">{project.key}</span>
        <h1 className="text-2xl font-semibold">{project.name}</h1>
      </div>
      <nav aria-label={t("nav")} className="mt-4 flex gap-5 border-b text-sm">
        <Link href={`/p/${project.key}/board`} className="pb-2">{t("board")}</Link>
        <Link href={`/p/${project.key}/tickets`} className="pb-2">{t("tickets")}</Link>
        <Link href={`/p/${project.key}/modules`} className="pb-2">{t("modules")}</Link>
        {project.role === "admin" && (
          <Link href={`/p/${project.key}/settings`} className="pb-2">{t("settings")}</Link>
        )}
      </nav>
      <div className="mt-6">{children}</div>
    </div>
  );
}
