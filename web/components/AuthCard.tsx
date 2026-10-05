import Icon from "./Icon";
import { panel } from "@/lib/ui";

// The centered card of the sign-in and password setup pages.
export default function AuthCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="mb-5 flex items-center justify-center gap-2">
          <Icon name="logo" className="size-7 text-accent" />
          <span className="text-2xl font-semibold">Zettra</span>
        </div>
        <div className={`${panel} p-6 shadow-sm`}>
          <h1 className="mb-5 text-lg font-semibold">{title}</h1>
          {children}
        </div>
      </div>
    </main>
  );
}
