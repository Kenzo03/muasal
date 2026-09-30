"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { api } from "@/lib/api";
import { useProblemText } from "@/lib/problem";
import { button, cx, field } from "@/lib/ui";

// "Run backup now": the backup service picks it up within 30 seconds (§19.4).
// While a request waits, the page watches for the new dump and lists it when
// it lands (MSL-26). newest names the latest dump as file@time, since a second
// backup in the same minute keeps the file name.
export default function RunBackup({ requested, newest }: { requested: boolean; newest: string }) {
  const t = useTranslations("backups");
  const problemText = useProblemText();
  const router = useRouter();
  const [state, setState] = useState<"idle" | "busy" | "waiting" | "slow" | "done">(requested ? "waiting" : "idle");
  const [error, setError] = useState("");
  const [file, setFile] = useState("");
  const before = useRef(newest);
  const waiting = state === "waiting" || state === "slow";
  useEffect(() => {
    if (!waiting) return;
    const started = Date.now();
    const timer = setInterval(async () => {
      const { data } = await api.GET("/admin/backups");
      const first = data?.items[0];
      if (first && `${first.file}@${first.created_at}` !== before.current) {
        setFile(first.file);
        setState("done");
        router.refresh();
      } else if (Date.now() - started > 15 * 60_000) {
        clearInterval(timer); // ponytail: stop watching; a reload shows a late backup
      } else if (Date.now() - started > 2 * 60_000) {
        setState("slow");
      }
    }, 5000);
    return () => clearInterval(timer);
  }, [waiting, router]);
  const status = { waiting: t("requested"), slow: t("slow"), done: t("finished", { file }) }[state as string];
  return (
    <div className="ml-auto flex items-center gap-3">
      {status && <span role="status" className="text-[13px] text-muted">{status}</span>}
      {error && <span role="alert" className={field.error}>{error}</span>}
      <button
        type="button"
        disabled={state === "busy" || waiting}
        className={cx(button.primary)}
        onClick={async () => {
          setState("busy");
          setError("");
          before.current = newest;
          const { error } = await api.POST("/admin/backups/run");
          if (error) {
            setError(problemText(error));
            return setState("idle");
          }
          setState("waiting");
        }}
      >
        {t("runNow")}
      </button>
    </div>
  );
}
