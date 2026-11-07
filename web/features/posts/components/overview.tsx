"use client";

import Link from "next/link";
import { ArrowRight, CalendarClock, CheckCircle2, FileText, Plus, RefreshCcw, Users } from "lucide-react";
import { useAccounts } from "@/features/accounts/hooks/use-accounts";
import { usePosts, useUpcomingPosts } from "@/features/posts/hooks/use-posts";
import { StatusBadge } from "@/components/posts/status-badge";
import { Button, buttonVariants } from "@/components/ui/button";
import type { PostResponse } from "@/types/post";

function PostRow({ post, upcoming = false }: { post: PostResponse; upcoming?: boolean }) {
  const date = upcoming ? post.scheduled_at : post.created_at;
  return (
    <Link href={`/posts/${post.id}`} className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-2 rounded-lg p-3 transition-colors sm:flex hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-ring">
      <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted/40"><FileText className="size-4 text-muted-foreground" /></div>
      <div className="min-w-0 flex-1">
        <p className="line-clamp-2 wrap-anywhere text-sm font-medium sm:truncate">{post.title || post.caption || "Untitled post"}</p>
        <p className="mt-1 wrap-anywhere text-xs sm:truncate text-muted-foreground">{Array.from(new Set(post.targets?.map(t => t.platform === "twitter" ? "X" : t.platform) ?? [])).join(" · ") || "No destinations"}{date ? ` · ${new Date(date).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })}` : ""}</p>
      </div>
      <div className="col-start-2"><StatusBadge status={post.status} /></div>
      <ArrowRight className="hidden size-4 text-muted-foreground sm:block" />
    </Link>
  );
}

export function Overview() {
  const recent = usePosts({ limit: 6 }, 30_000);
  const published = usePosts({ status: "PUBLISHED", limit: 1 }, 30_000);
  const scheduled = usePosts({ status: "SCHEDULED", limit: 1 }, 30_000);
  const failed = usePosts({ status: "FAILED", limit: 1 }, 30_000);
  const upcoming = useUpcomingPosts(5);
  const accounts = useAccounts();
  const metrics = [
    { label: "Total posts", query: recent, href: "/posts", icon: FileText },
    { label: "Published", query: published, href: "/posts?status=PUBLISHED", icon: CheckCircle2 },
    { label: "Scheduled", query: scheduled, href: "/posts?status=SCHEDULED", icon: CalendarClock },
    { label: "Failed posts", query: failed, href: "/posts?status=FAILED", icon: RefreshCcw },
  ];
  return (
    <div className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div><h1 className="text-2xl font-semibold tracking-tight">Overview</h1><p className="mt-1 text-sm text-muted-foreground">Your publishing workspace at a glance.</p></div>
        <Link href="/posts/new" className={buttonVariants({ className: "hidden lg:inline-flex" })}><Plus className="mr-2 size-4" />Create post</Link>
      </div>
      <div className="grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4">
        {metrics.map(({ label, query, href, icon: Icon }) => (
          <Link key={label} href={href} className="rounded-xl border bg-card p-3 transition-colors hover:bg-muted/30 sm:p-5">
            <div className="flex items-center justify-between gap-1 text-xs text-muted-foreground sm:text-sm"><span>{label}</span><Icon className="size-4 shrink-0" /></div>
            <p className="mt-2 text-2xl font-semibold tabular-nums sm:mt-4 sm:text-3xl">{query.isPending ? "…" : query.isError ? "—" : query.data?.total ?? 0}</p>
            <p className="mt-2 text-xs text-muted-foreground">View posts <ArrowRight className="ml-1 inline size-3" /></p>
          </Link>
        ))}
      </div>
      {(published.isError || scheduled.isError || failed.isError) && <p role="alert" className="text-sm text-destructive">Some counts could not load. <button className="underline" onClick={() => { void published.refetch(); void scheduled.refetch(); void failed.refetch(); }}>Retry counts</button></p>}
      <div className="grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
        <section className="rounded-xl border bg-card p-4 sm:p-5">
          <div className="mb-4 flex items-center justify-between"><h2 className="font-semibold">Recent posts</h2><Link href="/posts" className="text-xs text-muted-foreground hover:underline">View all posts →</Link></div>
          {recent.isPending ? <p role="status" className="py-12 text-center text-sm text-muted-foreground">Loading recent posts…</p> : recent.isError ? <div role="alert" className="py-8 text-center"><p className="mb-3 text-sm">Could not load recent posts.</p><Button variant="outline" size="sm" onClick={() => recent.refetch()}>Try again</Button></div> : recent.data?.posts?.length ? <div className="divide-y">{recent.data.posts.map(post => <PostRow key={post.id} post={post} />)}</div> : <div className="rounded-lg border border-dashed px-6 py-12 text-center"><FileText className="mx-auto mb-3 size-8 text-muted-foreground" /><h3 className="font-medium">Your next idea starts here</h3><p className="mb-5 mt-2 text-sm text-muted-foreground">Create a post and publish to your connected platforms, or schedule it for later.</p><Link href="/posts/new" className={buttonVariants({ size: "sm" })}>Create your first post</Link></div>}
        </section>
        <div className="space-y-6">
          <section className="rounded-xl border bg-card p-5">
            <div className="mb-4 flex items-center justify-between"><h2 className="font-semibold">Up next</h2><Link href="/calendar" className="text-xs text-muted-foreground hover:underline">Open calendar →</Link></div>
            {upcoming.isPending ? <p role="status" className="py-6 text-sm text-muted-foreground">Loading schedule…</p> : upcoming.isError ? <div role="alert"><p className="mb-2 text-sm">Could not load scheduled posts.</p><Button size="sm" variant="outline" onClick={() => upcoming.refetch()}>Try again</Button></div> : upcoming.data?.posts?.length ? <div className="divide-y">{upcoming.data.posts.map(post => <PostRow key={post.id} post={post} upcoming />)}</div> : <div className="py-5 text-sm text-muted-foreground"><CalendarClock className="mb-3 size-6" /><p>No posts scheduled yet.</p><Link href="/posts/new" className="mt-3 inline-block font-medium text-foreground hover:underline">Schedule a post →</Link></div>}
          </section>
          <section className="rounded-xl border bg-card p-5">
            <div className="flex items-center gap-2"><Users className="size-4" /><h2 className="font-semibold">Connected accounts</h2></div>
            {accounts.isPending ? <p role="status" className="mt-4 text-sm text-muted-foreground">Loading accounts…</p> : accounts.isError ? <div role="alert" className="mt-4"><p className="mb-2 text-sm">Could not load accounts.</p><Button size="sm" variant="outline" onClick={() => accounts.refetch()}>Try again</Button></div> : <><p className="mt-3 text-sm text-muted-foreground">{accounts.data?.accounts?.length ?? 0} accounts connected to your workspace.</p><div className="mt-4 space-y-2">{accounts.data?.accounts?.map(account => <Link href="/accounts" key={account.id} className="flex items-center justify-between gap-3 rounded-md border p-3 text-xs hover:bg-muted/40"><span className="min-w-0 truncate"><span className="font-medium capitalize">{account.platform === "twitter" ? "X" : account.platform}</span> · {account.platform_user_id}</span><span className={account.reconnect_required ? "shrink-0 text-amber-700" : "shrink-0 text-emerald-700"}>{account.reconnect_required ? "Reconnect" : "Connected"}</span></Link>)}</div><Link href="/accounts" className="mt-4 inline-block text-sm font-medium hover:underline">{accounts.data?.accounts?.length ? "Manage accounts" : "Connect your first account"} →</Link></>}
          </section>
        </div>
      </div>
    </div>
  );
}
