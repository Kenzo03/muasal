"use client";

import { useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { initials } from "@/components/Chips";
import Icon from "@/components/Icon";
import { matchProjects } from "@/lib/projects";
import { chip, cx, panel } from "@/lib/ui";

type Row = { key: string; name: string; description: string; role: "admin" | "member" | "viewer"; archived: boolean };

const roleLabel = { admin: "roleAdmin", member: "roleMember", viewer: "roleViewer" } as const;

// The all-projects list, filtered as you type by name or key.
export default function ProjectList({ projects, open }: { projects: Row[]; open: Record<string, number> }) {
  const t = useTranslations("projectList");
  const th = useTranslations("home");
  const ts = useTranslations("settings");
  const [find, setFind] = useState("");
  const shown = matchProjects(projects, find);
  return (
    <div className="flex max-w-4xl flex-col gap-3">
      <label className="flex h-10 max-w-md items-center gap-2.5 rounded-xl border border-line bg-white px-3.5 text-muted shadow-[0_1px_2px_rgba(43,36,32,0.04)] focus-within:border-accent">
        <Icon name="search" />
        <input
          value={find}
          onChange={(e) => setFind(e.target.value)}
          aria-label={t("find")}
          placeholder={t("find")}
          className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted"
        />
      </label>
      {shown.length === 0 ? (
        <p className="text-muted">{t("none")}</p>
      ) : (
        <ul aria-label={t("heading")} className={cx(panel, "flex flex-col gap-0.5 p-1.5")}>
          {shown.map((p) => (
            <li key={p.key}>
              <Link href={`/p/${p.key}/board`} className="flex flex-wrap items-center gap-3 rounded-xl px-3 py-3 text-ink no-underline hover:bg-paper hover:text-ink sm:flex-nowrap">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-[10px] bg-accent-soft text-xs font-extrabold text-accent-strong">
                  {initials(p.name)}
                </span>
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="text-sm font-bold">{p.name}</span>
                  <span className="truncate text-xs text-muted">
                    <span className="font-bold">{p.key}</span>
                    {p.description && ` · ${p.description}`}
                  </span>
                </span>
                {(open[p.key] ?? 0) > 0 && <span className={cx(chip, "bg-accent-soft text-accent-strong")}>{th("yourTickets", { count: open[p.key] })}</span>}
                {p.archived ? (
                  <span className={cx(chip, "bg-well text-muted")}>{t("archived")}</span>
                ) : (
                  <span className={cx(chip, "bg-well text-ink-soft")}>{ts(roleLabel[p.role])}</span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
