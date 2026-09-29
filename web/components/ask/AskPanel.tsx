"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import Icon from "@/components/Icon";
import { api } from "@/lib/api";
import type { Project } from "@/lib/problem";
import { button } from "@/lib/ui";
import type { Chip } from "./Answer";
import AskView from "./AskView";

// The Ask panel (FSD §10.1): a slide-over from the top bar on every page. It
// starts with the page's project, and on a node page with that node too.
export default function AskPanel({ project }: { project?: Project }) {
  const t = useTranslations("ask");
  const path = usePathname();
  const ref = useRef<HTMLDialogElement>(null);
  const [chips, setChips] = useState<Chip[] | null>(null);
  const [session, setSession] = useState(0); // a fresh thread each time it opens

  async function open() {
    const preset: Chip[] = project ? [{ kind: "project", id: project.id, label: project.key }] : [];
    const node = /^\/p\/[^/]+\/modules\/(\d+)/.exec(path)?.[1];
    if (node) {
      const { data } = await api.GET("/nodes/{id}", { params: { path: { id: Number(node) } } });
      if (data) preset.push({ kind: "node", id: data.node.id, label: [...data.path.map((p) => p.name), data.node.name].join(" › ") });
    }
    setChips(preset);
    setSession((n) => n + 1);
    ref.current?.showModal();
  }

  return (
    <>
      <button type="button" onClick={open} className={button.secondary}>
        <Icon name="sparkle" className="size-4 text-accent" />
        <span className="sr-only sm:not-sr-only">{t("openPanel")}</span>
      </button>
      <dialog
        ref={ref}
        aria-labelledby="ask-panel-title"
        onClick={(e) => {
          if (e.target === e.currentTarget) ref.current?.close(); // the backdrop
        }}
        className="ml-auto mr-0 h-dvh max-h-dvh w-[min(560px,100vw)] max-w-none overflow-y-auto bg-ground p-0 text-ink shadow-2xl backdrop:bg-ink/40"
      >
        <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-line bg-white px-4 py-3">
          <h2 id="ask-panel-title" className="text-base font-semibold">{t("panelTitle")}</h2>
          <Link href="/ask" onClick={() => ref.current?.close()} className="ml-auto text-[13px]">{t("openPage")}</Link>
          <button type="button" onClick={() => ref.current?.close()} aria-label={t("close")} className="rounded p-1 text-muted hover:bg-paper hover:text-ink">
            <Icon name="x" className="size-4" />
          </button>
        </div>
        <div className="p-4">{chips && <AskView key={session} chips={chips} compact />}</div>
      </dialog>
    </>
  );
}
