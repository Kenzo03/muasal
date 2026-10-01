"use client";

import { useEffect, useRef } from "react";
import Icon from "./Icon";

type Props = {
  title: React.ReactNode;
  labelledBy: string;
  closeLabel: string;
  onClose: () => void;
  footer: React.ReactNode;
  children: React.ReactNode;
};

// A form that slides in from the right over a list (the admin screens' create
// and edit). It is a modal <dialog>, like CloseDialog: Escape closes it, focus
// stays inside, and on close focus goes back to whatever opened it.
export default function SidePanel({ title, labelledBy, closeLabel, onClose, footer, children }: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    if (!ref.current?.open) ref.current?.showModal();
    return () => opener?.focus();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby={labelledBy}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      className="fixed inset-y-0 right-0 left-auto m-0 h-dvh max-h-none w-full max-w-[460px] border-0 border-l border-line bg-white p-0 text-ink shadow-[-12px_0_40px_rgba(43,36,32,0.10)] backdrop:bg-ink/15"
    >
      <div className="flex h-full flex-col">
        <div className="flex items-center gap-3 border-b border-line-soft px-5 py-4">
          <div className="min-w-0 flex-1">{title}</div>
          <button type="button" onClick={onClose} aria-label={closeLabel} className="inline-flex size-9 items-center justify-center rounded-[9px] text-muted hover:bg-paper hover:text-ink">
            <Icon name="x" />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">{children}</div>
        <div className="flex justify-end gap-2 border-t border-line-soft bg-paper px-5 py-3.5">{footer}</div>
      </div>
    </dialog>
  );
}
