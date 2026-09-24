import { getTranslations } from "next-intl/server";
import LoginForm from "./LoginForm";

export default async function LoginPage() {
  const t = await getTranslations("login");
  return (
    <main className="mx-auto mt-24 max-w-sm rounded-lg border bg-white p-8 shadow-sm">
      <h1 className="mb-6 text-xl font-semibold">{t("title")}</h1>
      <LoginForm />
    </main>
  );
}
