"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";

type Props = Omit<React.FormHTMLAttributes<HTMLFormElement>, "action" | "method" | "onSubmit" | "onChange"> & { action: string };

// AutoApplyForm is the filter bar's GET form, applied as it changes: a select
// at once, the search box after half a second without typing (or on Enter).
// It replaces the URL in place, so the page keeps its focus and scroll, and
// the view stays a URL people can share.
export default function AutoApplyForm({ action, children, ...props }: Props) {
  const router = useRouter();
  const typing = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(typing.current), []);

  function apply(form: HTMLFormElement) {
    clearTimeout(typing.current);
    const query = new URLSearchParams();
    for (const [name, value] of new FormData(form)) {
      if (typeof value === "string" && value !== "") query.append(name, value);
    }
    const qs = query.toString();
    router.replace(qs ? `${action}?${qs}` : action, { scroll: false });
  }

  return (
    <form
      method="get"
      action={action}
      {...props}
      onSubmit={(e) => {
        e.preventDefault();
        apply(e.currentTarget);
      }}
      onChange={(e) => {
        const field = e.target;
        if (!(field instanceof HTMLInputElement || field instanceof HTMLSelectElement) || !field.name) return; // not a filter: the phone's show-filters toggle
        const form = e.currentTarget;
        if (field instanceof HTMLInputElement && field.type === "text") {
          clearTimeout(typing.current);
          typing.current = setTimeout(() => apply(form), 500);
        } else {
          apply(form);
        }
      }}
    >
      {children}
    </form>
  );
}
