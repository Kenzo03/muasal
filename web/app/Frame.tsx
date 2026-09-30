"use client";

import { createContext, useContext, useEffect, useLayoutEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { useTranslations } from "next-intl";
import type { Project, User } from "@/lib/problem";
import { active } from "@/lib/projects";
import Sidebar from "./Sidebar";
import TopBar from "./TopBar";

// The project a page belongs to, read from its URL: /p/{key}/…, a ticket such as
// /t/HRIS-231, a document such as /documents/HRIS-DOC1 or a note such as /notes/HRIS-DN7.
function currentKey(path: string): string | undefined {
  const m =
    path.match(/^\/p\/([^/]+)/) ??
    path.match(/^\/t\/(.+)-\d+$/) ??
    path.match(/^\/documents\/(.+)-DOC\d+$/i) ??
    path.match(/^\/notes\/(.+)-DN\d+$/i);
  return m?.[1].toUpperCase();
}

const PageProject = createContext<(key?: string) => void>(() => {});

// Pages whose URL does not name their project, such as a tree draft or a
// summary, render <ProjectOf> so the sidebar still shows that project.
export function ProjectOf({ projectKey }: { projectKey: string }) {
  const set = useContext(PageProject);
  // Before paint, so client navigation does not flash the project list.
  useLayoutEffect(() => {
    set(projectKey);
    return () => set(undefined);
  }, [projectKey, set]);
  return null;
}

// The signed-in frame of the Terakota Lembut design: the sidebar, the top bar and
// the page. From md up the sidebar can shrink to a rail of icons; the choice
// lives in the `nav` cookie so the server renders it the same way. On phones the
// sidebar is a drawer behind the top bar's menu button.
export default function Frame({ me, projects, rail: railCookie, recent: recentCookie, children }: {
  me: User;
  projects: Project[];
  rail: boolean;
  recent: string[];
  children: React.ReactNode;
}) {
  const t = useTranslations("nav");
  const path = usePathname();
  const [pageKey, setPageKey] = useState<string>();
  const project = projects.find((p) => p.key === (currentKey(path) ?? pageKey));
  const inUse = active(projects); // an archived project shows only while open (MSL-64)
  const [rail, setRail] = useState(railCookie);
  const [drawer, setDrawer] = useState(false);
  const [recent, setRecent] = useState(recentCookie);

  // The last five projects opened, for the sidebar outside a project; a cookie,
  // so the server draws the same list.
  const openKey = project?.key;
  useEffect(() => {
    if (!openKey) return;
    setRecent((keys) => {
      const next = [openKey, ...keys.filter((k) => k !== openKey)].slice(0, 5);
      document.cookie = `recent=${next.join(",")}; path=/; max-age=31536000; samesite=lax`;
      return next;
    });
  }, [openKey]);

  useEffect(() => {
    if (!drawer) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setDrawer(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [drawer]);

  function toggleRail() {
    document.cookie = rail ? "nav=; path=/; max-age=0" : "nav=rail; path=/; max-age=31536000; samesite=lax";
    setRail(!rail);
  }

  // Sign-in and setup (the pages proxy.ts opens without a session) stand alone
  // even when a session is open, as when an admin opens someone's setup link.
  if (/^\/(login|setup)(\/|$)/.test(path)) return children;

  return (
    <div className="flex min-h-screen">
      {drawer && (
        <button type="button" aria-label={t("closeMenu")} onClick={() => setDrawer(false)} className="fixed inset-0 z-30 cursor-default bg-ink/30 md:hidden" />
      )}
      <Sidebar
        me={me}
        projects={inUse}
        project={project}
        recent={recent}
        rail={rail}
        drawer={drawer}
        onNavigate={() => setDrawer(false)}
        onToggleRail={toggleRail}
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar me={me} projects={inUse} project={project} recent={recent} drawer={drawer} onMenu={() => setDrawer(true)} />
        <PageProject.Provider value={setPageKey}>{children}</PageProject.Provider>
      </div>
    </div>
  );
}
