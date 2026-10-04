import { redirect } from "next/navigation";
import { getTranslations } from "next-intl/server";
import AuthCard from "@/components/AuthCard";
import { getMe, serverApi } from "@/lib/server-api";
import Approve from "./Approve";

type Params = Partial<Record<"client_id" | "redirect_uri" | "response_type" | "code_challenge" | "code_challenge_method" | "state" | "resource", string>>;

// MCP spec: an AI agent asks to act as the signed-in user. Nothing is issued
// until they click Allow; the API checks the request again then.
export default async function AuthorizePage({ searchParams }: { searchParams: Promise<Params> }) {
  const p = await searchParams;
  const me = await getMe();
  if (!me) redirect(`/login?next=${encodeURIComponent(`/oauth/authorize?${new URLSearchParams(p as Record<string, string>)}`)}`);
  const t = await getTranslations("oauth");
  const { data: client } = p.client_id
    ? await (await serverApi()).GET("/oauth/clients/{id}", { params: { path: { id: p.client_id } } })
    : { data: undefined };
  const valid = client && p.redirect_uri && client.redirect_uris.includes(p.redirect_uri) &&
    p.response_type === "code" && p.code_challenge_method === "S256" && p.code_challenge;
  return (
    <AuthCard title={t("title")}>
      {valid ? (
        <Approve
          client={client.name}
          host={new URL(p.redirect_uri!).host}
          me={{ name: me.name, email: me.email }}
          request={{ client_id: p.client_id!, redirect_uri: p.redirect_uri!, code_challenge: p.code_challenge!, code_challenge_method: "S256", state: p.state, resource: p.resource }}
        />
      ) : (
        <p role="alert">{t("invalid")}</p>
      )}
    </AuthCard>
  );
}
