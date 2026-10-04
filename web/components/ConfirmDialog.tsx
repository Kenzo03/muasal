"use client";

import { useEffect, useRef, useState } from "react";
import Icon from "./Icon";
import { button, field } from "@/lib/ui";

type Props = {
  title: string;
  body: string;
  action: string;
  cancelLabel: string;
  onConfirm: () => Promise<string | undefined>; // error text, or undefined when done
  onCancel: () => void;
};

// Asks before an action that can't be taken back quietly (reset password,
// disable, archive). The action stays open on failure and shows why.
export default function ConfirmDialog({ title, body, action, cancelLabel, onConfirm, onCancel }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (!ref.current?.open) ref.current?.showModal();
    return () => opener?.focus();
  }, []);
  async function confirm() {
    setBusy(true);
    const problem = await onConfirm();
    setBusy(false);
    if (problem) setError(problem);
  }
  return (
    <dialog
      ref={ref}
      role="alertdialog"
      aria-labelledby="confirm-title"
      aria-describedby="confirm-body"
      onCancel={(e) => {
        e.preventDefault();
        onCancel();
      }}
      className="m-auto w-[min(420px,calc(100vw-2rem))] rounded-2xl bg-white p-0 text-ink shadow-[0_24px_64px_rgba(43,36,32,0.22)] backdrop:bg-ink/35"
    >
      <div className="flex flex-col gap-2.5 px-6 pb-1 pt-5">
        <span className="inline-flex size-10 items-center justify-center rounded-xl bg-danger-soft text-danger">
          <Icon name="warning" className="size-[18px]" />
        </span>
        <h2 id="confirm-title" className="text-[17px] font-extrabold">{title}</h2>
        <p id="confirm-body" className="text-sm leading-relaxed text-ink-soft">{body}</p>
        {error && <p role="alert" className={field.error}>{error}</p>}
      </div>
      <div className="flex justify-end gap-2 px-6 pb-5 pt-4">
        <button type="button" className={button.secondary} onClick={onCancel}>{cancelLabel}</button>
        <button type="button" className={button.danger} onClick={confirm} disabled={busy}>{action}</button>
      </div>
    </dialog>
  );
}
