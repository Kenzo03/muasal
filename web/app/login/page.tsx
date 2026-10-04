import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import AuthCard from "@/components/AuthCard";
import { getMe } from "@/lib/server-api";
import { safeNext } from "@/lib/next";
import LoginForm from "./LoginForm";

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ email?: string; next?: string }> }) {
  const { email, next } = await searchParams;
  // Already signed in, as in a tab left here that reloads: go on. Switching
  // accounts starts with signing out.
  if (await getMe()) redirect(safeNext(next));
  const t = await getTranslations("login");
  return (
    <AuthCard title={t("title")}>
      <LoginForm email={email} next={next} />
    </AuthCard>
  );
}
