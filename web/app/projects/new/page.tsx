import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getMe } from "@/lib/server-api";
import NewProjectForm from "./NewProjectForm";

export default async function NewProjectPage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("newProject");
  if (!me.is_admin) return <main className="p-8">{t("adminsOnly")}</main>;
  return (
    <main className="mx-auto max-w-xl p-8">
      <h1 className="mb-6 text-2xl font-semibold">{t("title")}</h1>
      <NewProjectForm />
    </main>
  );
}
