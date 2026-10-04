"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { button, cx, field } from "@/lib/ui";

type Request = { client_id: string; redirect_uri: string; code_challenge: string; code_challenge_method: string; state?: string; resource?: string };

export default function Approve({ client, host, me, request }: { client: string; host: string; me: { name: string; email: string }; request: Request }) {
  const t = useTranslations("oauth");
  const [readOnly, setReadOnly] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function answer(allow: boolean) {
    setBusy(true);
    const { data } = await api.POST("/oauth/approve", { body: { ...request, read_only: readOnly, allow } });
    if (!data) {
      setBusy(false);
      setError(t("failed"));
      return;
    }
    window.location.assign(data.redirect_url);
  }

  return (
    <div className="flex flex-col gap-3.5">
      <p className="font-medium">{t("asks", { client })}</p>
      <p className="text-sm">{t("account", me)}</p>
      <p className="text-sm">{t("can")}</p>
      <p className="text-sm">{t("returnsTo", { host })}</p>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={readOnly} onChange={(e) => setReadOnly(e.target.checked)} />
        {t("readOnly")}
      </label>
      {error && <p role="alert" className={field.error}>{error}</p>}
      <div className="flex gap-2">
        <button type="button" disabled={busy} onClick={() => answer(false)} className={cx(button.secondary, "h-9 flex-1")}>{t("deny")}</button>
        <button type="button" disabled={busy} onClick={() => answer(true)} className={cx(button.primary, "h-9 flex-1")}>{t("allow")}</button>
      </div>
      <p className="text-xs text-muted">{t("revoke")}</p>
    </div>
  );
}
