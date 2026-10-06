"use client";

import { createContext, useContext, useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { useLiveRefresh } from "@/components/LiveEvents";
import { button } from "@/lib/ui";

const Editing = createContext<((delta: number) => void) | null>(null);

// useEditing tells the ticket page an editor is open, so live updates wait
// for it instead of changing the page under the user.
export function useEditing(active: boolean) {
  const count = useContext(Editing);
  useEffect(() => {
    if (!active || !count) return;
    count(1);
    return () => count(-1);
  }, [active, count]);
}

// TicketLive refreshes the ticket page when this ticket changes elsewhere
// (spec: live ticket updates). While an editor is open it holds the refresh
// and says the ticket changed; closing the editor refreshes, and "Reload
// now" drops the edit.
export default function TicketLive({ ticketId, children }: { ticketId: number; children: React.ReactNode }) {
  const t = useTranslations("ticket");
  const [editors, setEditors] = useState(0);
  const [count] = useState(() => (delta: number) => setEditors((n) => n + delta));
  const stale = useLiveRefresh({ ticketId, paused: editors > 0 });
  return (
    <Editing.Provider value={count}>
      {stale && (
        <p role="status" className="mx-4 mt-3 flex items-center gap-3 rounded-xl bg-accent-soft px-4 py-2 text-sm md:mx-5">
          {t("liveUpdated")}
          <button type="button" className={button.secondary} onClick={() => window.location.reload()}>
            {t("liveReload")}
          </button>
        </p>
      )}
      {children}
    </Editing.Provider>
  );
}
