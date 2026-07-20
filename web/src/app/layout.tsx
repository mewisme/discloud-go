import type { Metadata } from "next";
import { Geist_Mono, Inter } from "next/font/google";
import Link from "next/link";
import { Cloud } from "lucide-react";
import { Toaster } from "sonner";

import { HealthBanner } from "@/components/health-banner";
import { ThemeProvider } from "@/components/theme-provider";
import { ThemeToggle } from "@/components/theme-toggle";
import "./globals.css";
import { cn } from "@/lib/utils";

const inter = Inter({ subsets: ['latin'], variable: '--font-sans' });

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: {
    default: "DisCloud",
    template: "%s | DisCloud",
  },
  description: "Unlimited cloud storage backed by Discord attachments.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const apiOrigin = (
    process.env.API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    "http://localhost:8080"
  ).replace(/\/$/, "");

  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn("h-full", "antialiased", geistMono.variable, "font-sans", inter.variable)}
    >
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `self.__DISCLOUD_API__=${JSON.stringify(apiOrigin)}`,
          }}
        />
      </head>
      <body className="flex min-h-full flex-col bg-background font-sans text-foreground">
        <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
          <HealthBanner />
          <header className="sticky top-0 z-10 border-b border-border/60 bg-background/80 backdrop-blur">
            <div className="mx-auto flex h-14 w-full max-w-4xl items-center gap-6 px-4">
              <Link
                href="/"
                className="flex items-center gap-2 font-semibold tracking-tight"
              >
                <Cloud className="size-5 text-primary" aria-hidden />
                DisCloud
              </Link>
              <nav className="flex items-center gap-4 text-sm text-muted-foreground">
                <Link href="/" className="transition-colors hover:text-foreground">
                  Upload
                </Link>
                <Link href="/files" className="transition-colors hover:text-foreground">
                  Files
                </Link>
                <Link href="/docs" className="transition-colors hover:text-foreground">
                  API
                </Link>
              </nav>
              <div className="ml-auto">
                <ThemeToggle />
              </div>
            </div>
          </header>
          <main className="mx-auto w-full max-w-4xl flex-1 px-4 py-10">
            {children}
          </main>
          <footer className="border-t border-border/60 py-6 text-center text-xs text-muted-foreground">
            DisCloud — files are chunked and stored as Discord attachments.
          </footer>
          <Toaster richColors position="bottom-right" />
        </ThemeProvider>
      </body>
    </html>
  );
}
