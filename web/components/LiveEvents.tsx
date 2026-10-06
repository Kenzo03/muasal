"use client";

import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { components } from "@/lib/api-types";
import { refresher } from "@/lib/live";

type Notification = components["schemas"]["Notification"];
type TicketChange = number[] | "all";
type Listeners = { notes: Set<(n: Notification) => void>; tickets: Set<(c: TicketChange) => void> };

const Live = createContext<Listeners | null>(null);

// LiveEvents holds the tab's one event stream (spec: live ticket updates):
// the bell's notifications and, inside a project, which tickets changed. One
// stream per tab keeps plain-HTTP installs under the browser's six-connection
// limit.
export default function LiveEvents({ projectKey, children }: { projectKey?: string; children: React.ReactNode }) {
  const [listeners] = useState<Listeners>(() => ({ notes: new Set(), tickets: new Set() }));
  useEffect(() => {
    const es = new EventSource("/api/v1/events" + (projectKey ? `?project=${encodeURIComponent(projectKey)}` : ""));
    let opened = false;
    const all = () => listeners.tickets.forEach((f) => f("all"));
    es.onopen = () => {
      if (opened) all(); // a reconnect may have missed changes
      opened = true;
    };
    es.addEventListener("notification", (e) => {
      const n = JSON.parse((e as MessageEvent).data) as Notification;
      listeners.notes.forEach((f) => f(n));
    });
    es.addEventListener("tickets", (e) => {
      const { tickets } = JSON.parse((e as MessageEvent).data) as { tickets: number[] };
      listeners.tickets.forEach((f) => f(tickets));
    });
    es.addEventListener("resync", all);
    return () => es.close();
  }, [projectKey, listeners]);
  return <Live.Provider value={listeners}>{children}</Live.Provider>;
}

// useListen subscribes fn to one kind of event, always calling the latest fn.
function useListen<T>(pick: (l: Listeners) => Set<(v: T) => void>, fn: (v: T) => void) {
  const listeners = useContext(Live);
  const latest = useRef(fn);
  useEffect(() => {
    latest.current = fn;
  });
  useEffect(() => {
    if (!listeners) return;
    const set = pick(listeners);
    const h = (v: T) => latest.current(v);
    set.add(h);
    return () => {
      set.delete(h);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [listeners]);
}

export function useNotifications(fn: (n: Notification) => void) {
  useListen((l) => l.notes, fn);
}

export function useTicketChanges(fn: (c: TicketChange) => void) {
  useListen((l) => l.tickets, fn);
}

// useLiveRefresh refreshes the page's server data when its tickets change:
// all of the project's, or one ticket's. While paused it holds the refresh
// and returns stale = true; resuming runs it.
export function useLiveRefresh({ ticketId, paused = false }: { ticketId?: number; paused?: boolean } = {}): boolean {
  const router = useRouter();
  const routerRef = useRef(router);
  useEffect(() => {
    routerRef.current = router;
  });
  const [stale, setStale] = useState(false);
  const [r] = useState(() =>
    refresher(() => {
      setStale(false);
      routerRef.current.refresh();
    }),
  );
  useEffect(() => {
    r.pause(paused);
    if (!paused) setStale(false);
  }, [r, paused]);
  useEffect(() => {
    const onVisibility = () => r.visible(!document.hidden);
    onVisibility();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      r.dispose();
    };
  }, [r]);
  const pausedRef = useRef(paused);
  useEffect(() => {
    pausedRef.current = paused;
  });
  useTicketChanges((c) => {
    if (c !== "all" && ticketId !== undefined && !c.includes(ticketId)) return;
    if (pausedRef.current) setStale(true);
    r.signal();
  });
  return stale;
}

// LiveRefresh gives a server page live updates for its project's tickets.
export function LiveRefresh() {
  useLiveRefresh();
  return null;
}
