import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import Link from "next/link";
import { connection } from "next/server";
import { Cloud } from "lucide-react";

import { HealthBanner } from "@/components/health-banner";
import { AuthHeader } from "@/components/auth-header";
import { ThemeProvider } from "@/components/theme-provider";
import { ThemeToggle } from "@/components/theme-toggle";
import { buttonVariants } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import "./globals.css";
import { cn } from "@/lib/utils";

const GITHUB_REPO = "https://github.com/mewisme/discloud-go";

const geistSans = Geist({
  subsets: ["latin"],
  variable: "--font-sans",
});

const geistMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-geist-mono",
});

export const metadata: Metadata = {
  title: {
    default: "DisCloud",
    template: "%s | DisCloud",
  },
  description: "Unlimited cloud storage. Upload, share, and download files.",
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  await connection();
  const apiOrigin = (
    process.env.API_URL || "http://localhost:8080"
  ).replace(/\/$/, "");

  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn(
        "h-full antialiased font-sans",
        geistSans.variable,
        geistMono.variable,
      )}
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
          <TooltipProvider>
            <HealthBanner />
            <header className="sticky top-0 z-10 border-b border-border/60 bg-background/80 backdrop-blur">
              <div className="mx-auto flex h-14 w-full max-w-4xl items-center gap-3 px-4">
                <Link
                  href="/"
                  className="flex shrink-0 items-center gap-2 font-semibold tracking-tight"
                >
                  <Cloud className="size-5 text-primary" aria-hidden />
                  DisCloud
                </Link>
                <nav className="hidden items-center gap-4 text-sm text-muted-foreground sm:flex">
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
                <div className="ml-auto flex shrink-0 items-center gap-1 sm:gap-2">
                  <AuthHeader />
                  <a
                    href={GITHUB_REPO}
                    target="_blank"
                    rel="noopener noreferrer"
                    aria-label="GitHub repository"
                    className={cn(
                      buttonVariants({ variant: "ghost", size: "icon" }),
                      "text-muted-foreground",
                    )}
                  >
                    <svg
                      viewBox="0 0 24 24"
                      className="size-4 fill-current"
                      aria-hidden
                    >
                      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2Z" />
                    </svg>
                  </a>
                  <ThemeToggle />
                </div>
              </div>
              <nav className="flex items-center gap-4 border-t border-border/60 px-4 py-2 text-sm text-muted-foreground sm:hidden">
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
            </header>
            <main className="mx-auto w-full max-w-4xl flex-1 px-4 py-10">
              {children}
            </main>
            <footer className="border-t border-border/60 py-6 text-center text-xs text-muted-foreground">
              DisCloud — upload, share, and download files.{" "}
              <a
                href={GITHUB_REPO}
                target="_blank"
                rel="noopener noreferrer"
                className="underline-offset-4 hover:text-foreground hover:underline"
              >
                GitHub
              </a>
            </footer>
            <Toaster richColors position="bottom-right" />
          </TooltipProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
