import type { Metadata } from "next";
import { IBM_Plex_Mono, Plus_Jakarta_Sans } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { getLocale } from "next-intl/server";
import Shell from "./Shell";
import "./globals.css";

// next/font downloads the fonts once at build time and the app serves the files
// itself, so no page loads a font from outside (FSD §18: nothing leaves).
// Plus Jakarta Sans for all text; IBM Plex Mono only for code.
const sans = Plus_Jakarta_Sans({ subsets: ["latin"], variable: "--font-jakarta" });
const mono = IBM_Plex_Mono({ subsets: ["latin"], weight: ["500", "600"], variable: "--font-plex-mono" });

export const metadata: Metadata = { title: "Muasal" };

// modal is the parallel route of the create-ticket modal (@modal), which opens
// over the current page (FSD §8.3).
export default async function RootLayout({ children, modal }: Readonly<{ children: React.ReactNode; modal: React.ReactNode }>) {
  const locale = await getLocale();
  return (
    <html lang={locale} className={`${sans.variable} ${mono.variable}`}>
      <body className="min-h-screen font-sans">
        <NextIntlClientProvider>
          <Shell>{children}</Shell>
          {modal}
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
