import Link from "next/link";
import { CalendarDays, CirclePlay, Layers3, Send } from "lucide-react";
import { LandingActions } from "@/components/layout/landing-actions";
import { appName } from "@/lib/brand";

const features = [
  { icon: Layers3, title: "Bring your channels together", detail: "Connect YouTube, TikTok, and X to manage your publishing from one workspace." },
  { icon: CalendarDays, title: "Plan ahead", detail: "Choose when your posts go live and see your upcoming content in a calendar." },
  { icon: Send, title: "Follow every publish", detail: "Publish now or schedule for later, then check delivery status in Blink." },
];

export default function HomePage() {
  return (
    <div className="min-h-dvh bg-black text-white [&_a:focus-visible]:outline-2 [&_a:focus-visible]:outline-offset-4 [&_a:focus-visible]:outline-white">
      <header className="border-b border-white/10">
        <nav aria-label="Main navigation" className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-x-4 gap-y-2 px-5 py-4 sm:px-8">
          <Link href="/" className="inline-flex min-h-11 items-center gap-2 text-sm font-semibold sm:text-base">
            <span aria-hidden="true" className="size-2.5 rounded-full bg-white" />
            {appName}
          </Link>
          <LandingActions navigation />
        </nav>
      </header>

      <main className="mx-auto max-w-6xl px-5 sm:px-8">
        <section className="grid items-center gap-12 py-16 sm:py-24 lg:grid-cols-[1fr_0.9fr] lg:gap-16">
          <div>
            <p className="mb-5 font-mono text-xs tracking-[0.2em] text-white uppercase">Social publishing workspace</p>
            <h1 className="max-w-xl text-5xl font-semibold leading-[0.98] tracking-[-0.07em] sm:text-7xl">
              Plan your posts.<br />Publish on your time.
            </h1>
            <p className="mt-6 max-w-md text-base leading-7 text-neutral-400 sm:text-lg">
              {appName} helps creators prepare, schedule, and publish content to YouTube, TikTok, and X from one place.
            </p>
            <div className="mt-8 flex flex-wrap items-center gap-x-6 gap-y-3">
              <LandingActions />
              <a href="#how-it-works" className="inline-flex min-h-11 items-center text-sm text-neutral-200 underline decoration-white/30 underline-offset-4 hover:decoration-current">See how it works</a>
            </div>
          </div>
          <figure className="group relative isolate overflow-hidden rounded-3xl border border-white/10 bg-neutral-950 p-5 sm:p-8">
            <div aria-hidden="true" className="absolute inset-0 -z-10 bg-[radial-gradient(ellipse_at_top_right,#333333,transparent_70%)]" />
            <figcaption className="mb-5 font-mono text-xs tracking-widest text-neutral-400 uppercase">Publishing queue preview</figcaption>
            <div className="rounded-2xl border border-white/15 bg-black/85 p-4 shadow-xl motion-safe:transition-transform motion-safe:duration-500 motion-safe:group-hover:-translate-y-1 sm:p-5">
              <div className="relative mb-5 grid aspect-video place-items-center overflow-hidden rounded-xl bg-neutral-950 text-black">
                <svg viewBox="0 0 400 225" preserveAspectRatio="xMidYMid slice" className="absolute inset-0 size-full" aria-hidden="true">
                  <defs>
                    <radialGradient id="queue-glow">
                      <stop offset="0%" stopColor="#525252" />
                      <stop offset="100%" stopColor="#171717" />
                    </radialGradient>
                  </defs>
                  <rect width="400" height="225" fill="url(#queue-glow)" />
                  <g fill="none" stroke="white" strokeOpacity=".24">
                    <ellipse cx="200" cy="112.5" rx="182" ry="47" transform="rotate(-22 200 112.5)" />
                    <ellipse cx="200" cy="112.5" rx="143" ry="68" transform="rotate(26 200 112.5)" />
                    <circle cx="200" cy="112.5" r="86" />
                  </g>
                  <circle cx="74" cy="144" r="3" fill="white" />
                  <circle cx="326" cy="77" r="4" fill="white" />
                </svg>
                <CirclePlay aria-hidden="true" className="relative size-14 rounded-full bg-white p-2 motion-safe:transition-transform motion-safe:group-hover:scale-110" />
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-semibold">Studio notes</p>
                  <p className="mt-1 text-xs text-neutral-400">YouTube · Friday, 10:00 AM</p>
                </div>
                <span className="rounded-full bg-white/10 px-3 py-1 text-xs font-medium text-white">Scheduled</span>
              </div>
            </div>
          </figure>
        </section>

        <section id="how-it-works" aria-labelledby="workflow-heading" className="scroll-mt-8 border-y border-white/10 py-14 sm:py-16">
          <h2 id="workflow-heading" className="mb-9 text-3xl font-semibold tracking-tight sm:text-4xl">From draft to published</h2>
          <div className="grid gap-8 sm:grid-cols-3">
            {features.map(({ icon: Icon, title, detail }) => (
              <div key={title}>
                <Icon className="mb-4 size-6 text-white" aria-hidden="true" />
                <h3 className="mb-2 font-semibold">{title}</h3>
                <p className="max-w-xs text-sm leading-6 text-neutral-400">{detail}</p>
              </div>
            ))}
          </div>
        </section>

        <section aria-labelledby="platforms-heading" className="py-14 sm:py-20">
          <h2 id="platforms-heading" className="text-3xl font-semibold tracking-tight sm:text-4xl">What you can publish</h2>
          <p className="mt-3 leading-7 text-neutral-400">Connect only the accounts you want to publish to.</p>
          <div className="mt-8 grid gap-4 md:grid-cols-3">
            <div className="rounded-2xl border border-white/10 p-5">
              <h3 className="font-semibold">YouTube</h3>
              <p className="mt-2 text-sm leading-6 text-neutral-400">Upload your videos and Shorts. Blink uses your YouTube channel details to show which channel you connected and your upload permission to publish videos you provide, now or on your schedule.</p>
            </div>
            <div className="rounded-2xl border border-white/10 p-5">
              <h3 className="font-semibold">TikTok</h3>
              <p className="mt-2 text-sm leading-6 text-neutral-400">Publish videos with your caption and the privacy options available to your connected TikTok account.</p>
            </div>
            <div className="rounded-2xl border border-white/10 p-5">
              <h3 className="font-semibold">X</h3>
              <p className="mt-2 text-sm leading-6 text-neutral-400">Publish text posts or add images or a video to share with your followers on X.</p>
            </div>
          </div>
          <p className="mt-6 max-w-2xl text-sm leading-6 text-neutral-400">
            You can disconnect any connected account from Blink&apos;s Accounts page. Read our <Link href="/privacy" className="text-white underline underline-offset-4">Privacy Policy</Link> for details on how Blink handles connected account data.
          </p>
        </section>

        <section className="flex flex-wrap items-center justify-between gap-5 rounded-3xl border border-white/10 bg-white/5 p-6 sm:p-9">
          <h2 className="text-2xl font-semibold tracking-tight sm:text-3xl">Ready to plan your next post?</h2>
          <LandingActions />
        </section>
      </main>

      <footer className="mx-auto mt-14 flex max-w-6xl flex-wrap items-center justify-between gap-5 border-t border-white/10 px-5 py-7 text-sm text-neutral-400 sm:px-8">
        <Link href="/" className="font-medium text-white">{appName}</Link>
        <nav aria-label="Legal" className="flex flex-wrap gap-x-6 gap-y-2">
          <Link href="/privacy" className="hover:text-white hover:underline">Privacy Policy</Link>
          <Link href="/terms" className="hover:text-white hover:underline">Terms of Service</Link>
        </nav>
      </footer>
    </div>
  );
}
