import Link from "next/link";
import { SIGNUP_LEGAL_VERSION } from "@/lib/signup-legal-version";

export default function LegalLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-dvh">
      <header className="border-b">
        <div className="mx-auto flex max-w-3xl items-center justify-between gap-3 px-4 py-2 sm:px-6">
          <Link href="/" className="flex min-h-11 items-center text-xl font-semibold tracking-tight">
            Blink
          </Link>
          <Link href="/signup" className="flex min-h-11 items-center text-sm font-medium underline-offset-4 hover:underline">
            Create an account
          </Link>
        </div>
      </header>

      <main className="mx-auto max-w-3xl px-4 py-8 sm:px-6 sm:py-12">
        <p className="mb-2 text-sm text-muted-foreground">Version {SIGNUP_LEGAL_VERSION}</p>
        <article className="space-y-6 text-sm leading-7 text-muted-foreground sm:text-base [&_a]:wrap-anywhere [&_a]:font-medium [&_a]:text-foreground [&_a]:underline [&_a]:underline-offset-2 [&_h1]:text-3xl [&_h1]:font-semibold [&_h1]:tracking-tight [&_h1]:text-foreground [&_h2]:mb-2 [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:text-foreground [&_li+li]:mt-2 [&_ul]:list-disc [&_ul]:pl-5">
          {children}
        </article>
      </main>

      <footer className="border-t">
        <nav aria-label="Legal" className="mx-auto flex max-w-3xl flex-wrap gap-x-5 px-4 py-2 text-sm sm:px-6">
          <Link href="/terms" className="flex min-h-11 items-center underline-offset-4 hover:underline">Terms of Service</Link>
          <Link href="/privacy" className="flex min-h-11 items-center underline-offset-4 hover:underline">Privacy Policy</Link>
        </nav>
      </footer>
    </div>
  );
}
