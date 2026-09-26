"use client";

import { button, cx } from "@/lib/ui";

export default function PrintButton({ label }: { label: string }) {
  return (
    <button type="button" onClick={() => window.print()} className={cx(button.secondary, "print:hidden")}>{label}</button>
  );
}
