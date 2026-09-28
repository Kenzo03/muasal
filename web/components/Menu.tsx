"use client";

import { useEffect, useRef } from "react";
import { cx } from "@/lib/ui";

type Props = {
  label: string;
  summary: React.ReactNode;
  summaryClassName?: string;
  align?: "left" | "right";
  // "up" opens above the summary, for menus at the bottom of the sidebar.
  side?: "down" | "up";
  panelClassName?: string;
  id?: string;
  children: React.ReactNode;
};

// A dropdown on native <details>, so the summary is a real button for keyboards
// and screen readers. A click outside, Escape or following a link closes it.
export default function Menu({ label, summary, summaryClassName, align = "left", side = "down", panelClassName, id, children }: Props) {
  const ref = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const close = () => {
      if (ref.current) ref.current.open = false;
    };
    const onPointer = (e: PointerEvent) => {
      if (ref.current?.open && !ref.current.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && ref.current?.open) {
        close();
        ref.current.querySelector("summary")?.focus();
      }
    };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, []);
  return (
    <details ref={ref} id={id} className="relative">
      <summary aria-label={label} className={cx("cursor-pointer list-none", summaryClassName)}>
        {summary}
      </summary>
      <div
        onClick={(e) => {
          if ((e.target as HTMLElement).closest("a, button") && ref.current) ref.current.open = false;
        }}
        className={cx(
          "absolute z-30 min-w-60 overflow-hidden rounded-xl border border-line bg-white py-1.5 text-ink shadow-[0_12px_32px_rgba(43,36,32,0.12),0_2px_6px_rgba(43,36,32,0.06)]",
          side === "up" ? "bottom-full mb-2" : "top-full mt-1.5",
          align === "right" ? "right-0" : "left-0",
          panelClassName,
        )}
      >
        {children}
      </div>
    </details>
  );
}
