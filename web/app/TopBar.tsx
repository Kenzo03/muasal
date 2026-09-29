"use client";

import Form from "next/form";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import AskPanel from "@/components/ask/AskPanel";
import Bell from "@/components/Bell";
import Icon from "@/components/Icon";
import Menu from "@/components/Menu";
import type { Project, User } from "@/lib/problem";
import { matchProjects, recentFirst } from "@/lib/projects";
import { button } from "@/lib/ui";

type Props = { me: User; projects: Project[]; project?: Project; recent: string[]; drawer: boolean; onMenu: () => void };

// The light bar over every page: the sidebar's button on phones, search, Ask,
// New ticket and the bell.
export default function TopBar({ me, projects, project, recent, drawer, onMenu }: Props) {
  const t = useTranslations("nav");
  const router = useRouter();
  const search = useRef<HTMLInputElement>(null);
  const [find, setFind] = useState("");
  // Off a project page, New ticket asks which project (FSD §6.1), those opened last first.
  const creatable = recentFirst(projects.filter((p) => p.role !== "viewer"), recent);
  const newTicketKey = project ? (project.role !== "viewer" ? project.key : undefined) : creatable.length === 1 ? creatable[0].key : undefined;

  // `c` opens New ticket from anywhere (§6.1, §8.3) and `/` jumps to search,
  // unless the user is typing or a dialog is open. With several projects to
  // choose from, `c` opens the project menu instead. The shortcuts are
  // advertised only once the listener is attached, not before hydration.
  const [shortcut, setShortcut] = useState(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.key !== "c" && e.key !== "/") || e.ctrlKey || e.metaKey || e.altKey || e.repeat) return;
      const el = e.target as HTMLElement;
      if (el.closest("input, textarea, select, [contenteditable=true], dialog[open]") || document.querySelector("dialog[open]")) return;
      e.preventDefault();
      if (e.key === "/") return search.current?.focus();
      if (newTicketKey) return router.push(`/p/${newTicketKey}/tickets/new`);
      const menu = document.getElementById("new-ticket-menu") as HTMLDetailsElement | null;
      if (menu) {
        menu.open = true;
        menu.querySelector<HTMLElement>("input, a")?.focus(); // the search box, when the list is long
      }
    };
    document.addEventListener("keydown", onKey);
    setShortcut(true);
    return () => document.removeEventListener("keydown", onKey);
  }, [newTicketKey, router]);

  return (
    <header className="flex h-16 shrink-0 items-center gap-2 px-4 md:gap-3 md:px-5 print:hidden">
      <button
        type="button"
        onClick={onMenu}
        aria-label={t("openMenu")}
        aria-expanded={drawer}
        aria-controls="sidebar"
        className="flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-[11px] text-ink-soft hover:bg-well hover:text-ink md:hidden"
      >
        <Icon name="menu" className="size-5" />
      </button>
      <Form action="/search" role="search" className="min-w-0 flex-1 md:max-w-md">
        <label className="flex h-10 items-center gap-2.5 rounded-xl border border-line bg-white px-3.5 text-muted shadow-[0_1px_2px_rgba(43,36,32,0.04)] focus-within:border-accent">
          <Icon name="search" />
          <input
            ref={search}
            type="search"
            name="q"
            required
            aria-label={t("search")}
            aria-keyshortcuts={shortcut ? "/" : undefined}
            placeholder={t("searchPlaceholder")}
            className="min-w-0 flex-1 bg-transparent text-sm text-ink outline-none placeholder:text-muted"
          />
          {shortcut && <kbd className="hidden rounded-md border border-line bg-well px-1.5 font-sans text-xs font-bold text-ink-soft md:inline">/</kbd>}
        </label>
      </Form>
      <div className="ml-auto flex shrink-0 items-center gap-2">
        <AskPanel project={project} />
        {newTicketKey ? (
          <Link href={`/p/${newTicketKey}/tickets/new`} aria-label={t("newTicket")} aria-keyshortcuts={shortcut ? "c" : undefined} title={t("newTicketShortcut")} className={button.primary}>
            <Icon name="plus" />
            <span className="hidden sm:inline">{t("newTicket")}</span>
          </Link>
        ) : (
          !project &&
          creatable.length > 1 && (
            <Menu
              id="new-ticket-menu"
              align="right"
              label={t("newTicket")}
              panelClassName="w-72"
              summaryClassName={button.primary}
              summary={
                <>
                  <Icon name="plus" />
                  <span className="hidden sm:inline">{t("newTicket")}</span>
                  <Icon name="chevron" className="size-4" />
                </>
              }
            >
              <p className="px-3.5 pb-1 pt-1 text-xs font-semibold text-muted">{t("chooseProject")}</p>
              {creatable.length > 8 && (
                <label className="mx-2.5 mb-1 flex h-9 items-center gap-2 rounded-[10px] border border-line bg-paper px-2.5 text-muted focus-within:border-field focus-within:bg-white">
                  <Icon name="search" />
                  <input
                    value={find}
                    onChange={(e) => setFind(e.target.value)}
                    aria-label={t("findProject")}
                    placeholder={t("findProject")}
                    className="min-w-0 flex-1 bg-transparent text-[13.5px] text-ink outline-none placeholder:text-muted"
                  />
                </label>
              )}
              <div className="max-h-80 overflow-y-auto">
                {matchProjects(creatable, find).map((p) => (
                  <Link key={p.id} href={`/p/${p.key}/tickets/new`} className="flex items-baseline gap-2 px-3.5 py-2 text-sm text-ink no-underline hover:bg-paper hover:text-ink">
                    <span className="shrink-0 text-xs font-bold text-muted">{p.key}</span>
                    <span className="line-clamp-2">{p.name}</span>
                  </Link>
                ))}
              </div>
            </Menu>
          )
        )}
        <Bell browser={me.notify_prefs?.browser === true} />
      </div>
    </header>
  );
}
