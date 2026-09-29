"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTimeZone, useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { useProblemText, type AIMode, type AISettings, type AITestResult, type IndexStatus, type Problem } from "@/lib/problem";
import { button, cx, field, panel, sectionTitle, table } from "@/lib/ui";

type Endpoint = { url: string; model: string; key: string; clearKey: boolean };

// The Admin → AI form (FSD §13.4). AI is optional: Off runs keyword search
// only; Local uses the customer's own model server; Bring your own key sends
// questions and ticket excerpts to a cloud provider, after an acknowledgement
// (R-AI-1). Keys are write-only: the page shows only that one is saved.
export default function AIAdmin({ settings, status }: { settings: AISettings; status: IndexStatus }) {
  const t = useTranslations("ai");
  const locale = useLocale();
  const timeZone = useTimeZone();
  const problemText = useProblemText();
  const router = useRouter();
  const [mode, setMode] = useState<AIMode>(settings.mode);
  const [provider, setProvider] = useState(settings.provider);
  const [acknowledged, setAcknowledged] = useState(settings.acknowledged);
  const [chat, setChat] = useState<Endpoint>({ url: settings.chat.url, model: settings.chat.model, key: "", clearKey: false });
  const [embed, setEmbed] = useState<Endpoint>({ url: settings.embed.url, model: settings.embed.model, key: "", clearKey: false });
  const [tuning, setTuning] = useState(settings.tuning);
  const [embedDim, setEmbedDim] = useState(settings.embed_dim);
  const [test, setTest] = useState<AITestResult>();
  const [problem, setProblem] = useState<Problem>();
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const endpoint = (e: Endpoint) => ({
    url: e.url.trim(),
    model: e.model.trim(),
    ...(e.clearKey ? { api_key: "" } : e.key ? { api_key: e.key } : {}),
  });
  const body = (reindex = false) => ({
    mode,
    provider,
    acknowledged,
    chat: endpoint(chat),
    embed: endpoint(embed),
    tuning,
    ...(embedDim !== settings.embed_dim ? { embed_dim: embedDim } : {}),
    ...(reindex ? { reindex: true } : {}),
  });
  const fieldError = (name: string) => {
    const e = problem?.errors?.find((f) => f.field === name);
    return e && <p className={field.error}>{problemText({ ...problem!, errors: [e] })}</p>;
  };

  async function save(reindex = false) {
    setBusy(true);
    const { error } = await api.PUT("/admin/settings/ai", { body: body(reindex) });
    setBusy(false);
    if (error?.errors?.some((f) => f.code === "reindex_required") && !reindex) {
      // §13.4: a new embedding model re-embeds everything, so the admin confirms it.
      if (window.confirm(t("reindexConfirm", { chunks: status.total_chunks }))) return save(true);
      return;
    }
    if (error) {
      setProblem(error);
      setNotice("");
      return;
    }
    setProblem(undefined);
    setChat({ ...chat, key: "", clearKey: false });
    setEmbed({ ...embed, key: "", clearKey: false });
    setNotice(t("saved"));
    router.refresh();
  }

  async function testConnection() {
    setBusy(true);
    const { data, error } = await api.POST("/admin/ai/test", { body: body() });
    setBusy(false);
    if (error) return setProblem(error);
    setProblem(undefined);
    setTest(data);
    if (data.embed.dim && data.embed.dim !== embedDim) setEmbedDim(data.embed.dim);
  }

  async function reindex(scope: "all" | "failed") {
    if (scope === "all" && !window.confirm(t("reindexAllConfirm"))) return;
    const { data, error } = await api.POST("/admin/ai/reindex", { body: { scope } });
    if (error) return setProblem(error);
    setNotice(t("queued", { count: data.queued }));
    router.refresh();
  }

  const endpointFields = (name: "chat" | "embed", e: Endpoint, set: (e: Endpoint) => void, keySet: boolean) => (
    <fieldset className="flex flex-col gap-3" disabled={mode === "off"}>
      <legend className={cx(sectionTitle, "mb-2")}>{t(name)}</legend>
      <label className={field.label}>
        {t("url")}
        <input value={e.url} onChange={(ev) => set({ ...e, url: ev.target.value })} className={field.input} />
        {fieldError(`${name}.url`)}
      </label>
      <label className={field.label}>
        {t("model")}
        <input value={e.model} onChange={(ev) => set({ ...e, model: ev.target.value })} className={cx(field.input, "font-mono")} />
        {fieldError(`${name}.model`)}
      </label>
      <label className={field.label}>
        {t("apiKey")}
        <input
          type="password"
          autoComplete="off"
          value={e.key}
          onChange={(ev) => set({ ...e, key: ev.target.value, clearKey: false })}
          placeholder={keySet ? t("keySaved") : t("noKey")}
          className={field.input}
        />
        {fieldError(`${name}.api_key`)}
      </label>
      {keySet && (
        <label className="flex items-center gap-2 text-[13px]">
          <input type="checkbox" checked={e.clearKey} onChange={(ev) => set({ ...e, clearKey: ev.target.checked, key: "" })} className="size-4 accent-accent" />
          {t("removeKey")}
        </label>
      )}
    </fieldset>
  );

  const number = (name: keyof typeof tuning, step = 1) => (
    <label className={field.label}>
      {t(`tuning.${name}`)}
      <input
        type="number"
        step={step}
        value={tuning[name]}
        onChange={(e) => setTuning({ ...tuning, [name]: Number(e.target.value) })}
        className={cx(field.input, "w-32")}
      />
      {fieldError(name)}
    </label>
  );

  return (
    <div className="flex flex-col gap-4">
      <section aria-labelledby="ai-mode" className={cx(panel, "flex flex-col gap-3 p-4")}>
        <h2 id="ai-mode" className="text-sm font-semibold">{t("mode")}</h2>
        <div role="radiogroup" aria-labelledby="ai-mode" className="grid gap-2 md:grid-cols-3">
          {(["off", "local", "byok"] as const).map((m) => (
            <label key={m} className={cx("flex cursor-pointer flex-col gap-1 rounded border p-3 text-[13px]", mode === m ? "border-accent bg-accent-soft" : "border-line")}>
              <span className="flex items-center gap-2 font-semibold">
                <input type="radio" name="mode" value={m} checked={mode === m} onChange={() => setMode(m)} className="size-4 accent-accent" />
                {t(`modes.${m}`)}
              </span>
              <span className="text-muted">{t(`modes.${m}Hint`)}</span>
            </label>
          ))}
        </div>
        {mode === "byok" && (
          <div className="flex flex-col gap-2 rounded border border-warn-line bg-warn-soft p-3">
            <label className={field.label}>
              {t("provider")}
              <input value={provider} onChange={(e) => setProvider(e.target.value)} maxLength={100} placeholder="OpenAI" className={cx(field.input, "w-64")} />
              {fieldError("provider")}
            </label>
            <label className="flex items-start gap-2 text-[13px]">
              <input type="checkbox" checked={acknowledged} onChange={(e) => setAcknowledged(e.target.checked)} className="mt-0.5 size-4 accent-accent" />
              {t("acknowledge", { provider: provider.trim() || t("theProvider") })}
            </label>
            {fieldError("acknowledged")}
            {!settings.secret_key_set && <p className={field.hint}>{t("secretKeyMissing")}</p>}
          </div>
        )}
      </section>

      <section aria-label={t("endpoints")} className={cx(panel, "grid gap-6 p-4 md:grid-cols-2")}>
        {endpointFields("chat", chat, setChat, settings.chat.api_key_set)}
        {endpointFields("embed", embed, setEmbed, settings.embed.api_key_set)}
        <details className="md:col-span-2">
          <summary className="cursor-pointer text-[13px] text-link">{t("tuningTitle")}</summary>
          <div className="mt-3 flex flex-wrap gap-4">
            {number("context_tokens", 100)}
            {number("max_concurrent")}
            {number("temperature", 0.05)}
            {number("timeout_seconds")}
            {number("min_similarity", 0.05)}
            {number("exhaustive_max")}
          </div>
        </details>
      </section>

      <div className="flex flex-wrap items-center gap-2">
        <button type="button" disabled={busy} onClick={() => save()} className={button.primary}>{t("save")}</button>
        <button type="button" disabled={busy || mode === "off"} onClick={testConnection} className={button.secondary}>{t("test")}</button>
        {notice && <p role="status" className="text-[13px] text-ok">{notice}</p>}
        {problem && !problem.errors?.length && <p role="alert" className={field.error}>{problemText(problem)}</p>}
      </div>

      {test && (
        <section aria-label={t("testResult")} className={cx(panel, "grid gap-3 p-4 text-[13px] md:grid-cols-2")}>
          {(["chat", "embed"] as const).map((k) => (
            <div key={k} className="flex flex-col gap-1">
              <span className="font-semibold">{t(k)}: {test[k].ok ? t("ok", { ms: test[k].latency_ms }) : t("failed")}</span>
              {test[k].models && <span className="text-muted">{t("models")}: {test[k].models!.join(", ")}</span>}
              {test[k].dim && <span className="text-muted">{t("dimension", { dim: test[k].dim! })}</span>}
              {test[k].error && <span className="break-all text-danger">{test[k].error}</span>}
            </div>
          ))}
        </section>
      )}

      <section aria-labelledby="index-status" className={cx(panel, "flex flex-col gap-3 p-4")}>
        <div className="flex flex-wrap items-center gap-2">
          <h2 id="index-status" className="text-sm font-semibold">{t("indexStatus")}</h2>
          <button type="button" onClick={() => reindex("all")} className={cx(button.secondary, "ml-auto")}>{t("reindexAll")}</button>
        </div>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[13px]">
          <dt className="text-muted">{t("chunks")}</dt>
          <dd>{status.total_chunks}</dd>
          <dt className="text-muted">{t("pending")}</dt>
          <dd>{status.mode === "off" ? t("pendingOff", { count: status.pending_chunks }) : status.pending_chunks}</dd>
          <dt className="text-muted">{t("queuedJobs")}</dt>
          <dd>{status.queued_jobs}</dd>
          <dt className="text-muted">{t("lastIndexed")}</dt>
          <dd>{status.last_indexed_at ? dateTime(status.last_indexed_at, locale, timeZone) : "—"}</dd>
        </dl>
        {status.chunks_by_model.length > 0 && (
          <div className={table.wrap}>
            <table className={table.table}>
              <thead className={table.head}>
                <tr>
                  <th className={table.th}>{t("embedModel")}</th>
                  <th className={table.th}>{t("chunks")}</th>
                </tr>
              </thead>
              <tbody>
                {status.chunks_by_model.map((m) => (
                  <tr key={m.model} className={table.row}>
                    <td className={cx(table.td, m.model && "font-mono")}>{m.model || t("noVector")}</td>
                    <td className={table.td}>{m.chunks}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {status.failed_jobs.length > 0 && (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <h3 className={sectionTitle}>{t("failedJobs", { count: status.failed_jobs.length })}</h3>
              <button type="button" onClick={() => reindex("failed")} className={button.quiet}>{t("retryFailed")}</button>
            </div>
            <ul className="flex flex-col gap-1 text-xs">
              {status.failed_jobs.map((f) => (
                <li key={f.id} className="break-all text-muted">
                  {dateTime(f.at, locale, timeZone)} · #{f.ticket_id} · {f.error}
                </li>
              ))}
            </ul>
          </div>
        )}
      </section>
    </div>
  );
}
