import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe } from "@/lib/server-api";
import { panel } from "@/lib/ui";
import NewProjectForm from "./NewProjectForm";

export default async function NewProjectPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("newProject");
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
      </PageBar>
      <main className="p-4 md:p-5">
        {me.is_admin ? (
          <div className={`${panel} mx-auto max-w-xl p-5`}>
            <NewProjectForm />
          </div>
        ) : (
          <p className="text-muted">{t("adminsOnly")}</p>
        )}
      </main>
    </>
  );
}
