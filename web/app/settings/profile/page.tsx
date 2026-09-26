import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import PageBar from "@/components/PageBar";
import { getMe } from "@/lib/server-api";
import { panel } from "@/lib/ui";
import NotifyPrefsForm from "./NotifyPrefsForm";
import ProfileForm from "./ProfileForm";

export default async function ProfilePage() {
  const me = await getMe();
  if (!me) redirect("/login");
  const t = await getTranslations("profile");
  return (
    <>
      <PageBar>
        <h1 className="text-base font-semibold">{t("title")}</h1>
        <span className="text-[13px] text-muted">{me.email}</span>
      </PageBar>
      <main className="p-4 md:p-5">
        <div className={`${panel} mx-auto max-w-lg p-5`}>
          <ProfileForm me={me} />
        </div>
        <div className={`${panel} mx-auto mt-4 max-w-lg p-5`}>
          <NotifyPrefsForm me={me} />
        </div>
      </main>
    </>
  );
}
