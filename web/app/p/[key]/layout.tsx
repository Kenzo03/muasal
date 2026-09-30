import { getTranslations } from "next-intl/server";
import { getProject } from "@/lib/server-api";
import { cx, panel } from "@/lib/ui";
import ArchiveButton from "./ArchiveButton";

// Every project page says when the project is archived: read-only until a
// project admin restores it (MSL-64).
export default async function ProjectLayout({ children, params }: { children: React.ReactNode; params: Promise<{ key: string }> }) {
  const { key } = await params;
  const project = await getProject(key);
  const t = await getTranslations("archive");
  return (
    <>
      {project?.archived_at && (
        <div role="status" className={cx(panel, "mx-4 mb-2 flex flex-wrap items-center gap-3 px-4 py-3 text-[13px] md:mx-5 print:hidden")}>
          <span className="flex-1">{t("banner")}</span>
          {project.can_restore && <ArchiveButton projectKey={key} name={project.name} archived />}
        </div>
      )}
      {children}
    </>
  );
}
