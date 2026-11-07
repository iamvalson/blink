"use client";

import { useCalendarPosts, useUpcomingPosts } from "@/features/posts/hooks/use-posts";
import { cancelSchedule } from "@/features/posts/api/create-post";
import { ReschedulePostDialog } from "@/components/posts/reschedule-post-dialog";
import { StatusBadge } from "@/components/posts/status-badge";
import type { PostResponse } from "@/types/post";
import {
  CalendarDays,
  CalendarClock,
  ChevronLeft,
  ChevronRight,
  Clock,
  MoreHorizontal,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { calendarQueryKey, upcomingQueryKey } from "@/features/posts/hooks/use-posts";

import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// ─── Helpers ────────────────────────────────────────────────────────────────

function startOfMonth(year: number, month: number) {
  return new Date(year, month, 1);
}

function endOfMonth(year: number, month: number) {
  return new Date(year, month + 1, 0, 23, 59, 59, 999);
}

function daysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate();
}

function isSameDay(a: Date, b: Date) {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

const MONTH_NAMES = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

function PlatformBadge({ platform }: { platform: string }) {
  return (
    <span className="rounded bg-muted px-1 py-0.5 text-[10px] font-medium text-muted-foreground">
      {platform}
    </span>
  );
}

// ─── Loading skeleton ────────────────────────────────────────────────────────

function LoadingSkeleton({ className }: { className?: string }) {
  return (
    <div className={`animate-pulse rounded-lg bg-muted ${className ?? ""}`} />
  );
}

// ─── Upcoming sidebar item ───────────────────────────────────────────────────

function UpcomingPostItem({
  post,
  onCancelSchedule,
  onReschedule,
}: {
  post: PostResponse;
  onReschedule: (post: PostResponse) => void;
  onCancelSchedule: (id: string) => void;
}) {
  const scheduledAt = post.scheduled_at ? new Date(post.scheduled_at) : null;
  const platforms = Array.from(
    new Set((post.targets ?? []).map((t) => t.platform)),
  );

  return (
    <div className="group flex items-start gap-3 rounded-lg border bg-card px-4 py-3 transition-colors hover:bg-muted/40">
      <div className="mt-0.5 flex shrink-0 flex-col items-center text-center">
        {scheduledAt ? (
          <>
            <span className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {MONTH_NAMES[scheduledAt.getMonth()].slice(0, 3)}
            </span>
            <span className="text-2xl font-bold leading-none tabular-nums">
              {scheduledAt.getDate()}
            </span>
          </>
        ) : (
          <Clock className="h-5 w-5 text-muted-foreground" />
        )}
      </div>

      <div className="min-w-0 flex-1">
        <Link href={`/posts/${post.id}`} className="block line-clamp-2 wrap-anywhere text-sm font-medium leading-snug hover:underline">
          {post.caption ?? post.title ?? "(no caption)"}
        </Link>
        <div className="mt-1.5 flex flex-wrap items-center gap-2">
          {scheduledAt && (
            <span className="text-xs text-muted-foreground">
              {scheduledAt.toLocaleTimeString(undefined, { timeStyle: "short" })}
            </span>
          )}
          <div className="flex flex-wrap gap-1">
            {platforms.map((p) => (
              <PlatformBadge key={p} platform={p} />
            ))}
          </div>
          <StatusBadge status={post.status} />
        </div>
      </div>

      <DropdownMenu>
        <DropdownMenuTrigger
          aria-label="Post options"
          className="flex size-11 shrink-0 items-center justify-center rounded hover:bg-muted lg:size-8"
        >
          <MoreHorizontal className="h-4 w-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {(post.status === "SCHEDULED" || post.status === "QUEUED") && (
            <DropdownMenuItem onClick={() => onReschedule(post)}>
              <CalendarClock className="mr-2 h-4 w-4" />
              Reschedule
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            className="text-destructive focus:text-destructive"
            onClick={() => onCancelSchedule(post.id)}
          >
            <Trash2 className="mr-2 h-4 w-4" />
            Cancel schedule
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

// ─── Calendar cell ───────────────────────────────────────────────────────────

function CalendarCell({
  day,
  year,
  month,
  posts,
  isToday,
  isCurrentMonth,
}: {
  day: number;
  year: number;
  month: number;
  posts: PostResponse[];
  isToday: boolean;
  isCurrentMonth: boolean;
}) {
  // Suppress unused warnings — kept for future per-day navigation
  void year; void month;

  const MAX_DOTS = 3;
  const overflow = posts.length - MAX_DOTS;

  return (
    <div
      className={[
        "relative flex min-w-0 min-h-[90px] flex-col rounded-lg p-2 text-sm transition-colors",
        isCurrentMonth ? "bg-card" : "bg-muted/20",
        isToday ? "ring-2 ring-primary ring-offset-1" : "border",
      ].join(" ")}
    >
      <span
        className={[
          "self-end rounded-full px-1.5 py-0.5 text-xs font-semibold tabular-nums",
          isToday
            ? "bg-primary text-primary-foreground"
            : isCurrentMonth
            ? "text-foreground"
            : "text-muted-foreground",
        ].join(" ")}
      >
        {day}
      </span>

      <div className="mt-1 flex flex-col gap-0.5 overflow-hidden">
        {posts.slice(0, MAX_DOTS).map((post) => {
          const platforms = Array.from(
            new Set((post.targets ?? []).map((t) => t.platform)),
          );
          return (
            <Link
              href={`/posts/${post.id}`}
              key={post.id}
              aria-label={`View details for ${post.caption ?? post.title ?? "post"}`}
              title={post.caption ?? post.title ?? "(no caption)"}
              className={[
                "flex items-center gap-1 truncate rounded px-1 py-0.5 text-left text-[11px] font-medium leading-tight disabled:cursor-default",
                post.status === "SCHEDULED"
                  ? "bg-primary/10 text-primary"
                  : "bg-muted text-muted-foreground",
              ].join(" ")}
            >
              <span className="max-w-[45%] shrink-0 truncate text-[9px] uppercase tracking-tight">
                {platforms.join("/")}
              </span>
              <span className="truncate">
                {post.caption ?? post.title ?? "(no caption)"}
              </span>
            </Link>
          );
        })}

        {overflow > 0 && (
          <span className="px-1 text-[10px] text-muted-foreground">
            +{overflow} more
          </span>
        )}
      </div>
    </div>
  );
}

// ─── Main page ───────────────────────────────────────────────────────────────

export default function CalendarPage() {
  const today = useMemo(() => new Date(), []);
  const [viewYear, setViewYear] = useState(today.getFullYear());
  const [viewMonth, setViewMonth] = useState(today.getMonth());
  const [reschedulingPost, setReschedulingPost] = useState<PostResponse | null>(null);

  const rangeFrom = useMemo(() => startOfMonth(viewYear, viewMonth), [viewYear, viewMonth]);
  const rangeTo = useMemo(() => endOfMonth(viewYear, viewMonth), [viewYear, viewMonth]);

  const { data: calendarPosts = [], isLoading: isCalendarLoading, isError: isCalendarError, refetch } =
    useCalendarPosts(rangeFrom, rangeTo);

  const { data: upcomingData, isLoading: isUpcomingLoading } =
    useUpcomingPosts(20, 0);

  const queryClient = useQueryClient();

  const handleCancelSchedule = useCallback(
    async (postId: string) => {
      try {
        await cancelSchedule(postId);
        toast.success("Schedule cancelled.");
        queryClient.invalidateQueries({
          queryKey: calendarQueryKey(rangeFrom.toISOString(), rangeTo.toISOString()),
        });
        queryClient.invalidateQueries({ queryKey: upcomingQueryKey(20, 0) });
        queryClient.invalidateQueries({ queryKey: ["posts"] });
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to cancel schedule.";
        toast.error(msg);
      }
    },
    [queryClient, rangeFrom, rangeTo],
  );

  // Build a lookup: day-of-month → posts
  const postsByDay = useMemo(() => {
    const map: Record<number, PostResponse[]> = {};
    for (const post of calendarPosts) {
      const date = post.scheduled_at
        ? new Date(post.scheduled_at)
        : post.created_at
        ? new Date(post.created_at)
        : null;
      if (!date) continue;
      if (date.getFullYear() !== viewYear || date.getMonth() !== viewMonth) continue;
      const d = date.getDate();
      if (!map[d]) map[d] = [];
      map[d].push(post);
    }
    return map;
  }, [calendarPosts, viewYear, viewMonth]);

  const firstDayOfWeek = new Date(viewYear, viewMonth, 1).getDay();
  const totalDays = daysInMonth(viewYear, viewMonth);

  const prevMonth = () => {
    if (viewMonth === 0) { setViewYear((y) => y - 1); setViewMonth(11); }
    else setViewMonth((m) => m - 1);
  };
  const nextMonth = () => {
    if (viewMonth === 11) { setViewYear((y) => y + 1); setViewMonth(0); }
    else setViewMonth((m) => m + 1);
  };

  const upcomingPosts = upcomingData?.posts ?? [];

  return (
    <div className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
      {reschedulingPost && (
        <ReschedulePostDialog
          key={reschedulingPost.id}
          post={reschedulingPost}
          onClose={() => setReschedulingPost(null)}
        />
      )}
      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-4 sm:items-center">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Content Calendar</h1>
          <p className="text-sm text-muted-foreground">
            View and manage your scheduled posts.
          </p>
        </div>
        <Link href="/posts/new" className={buttonVariants({ className: "hidden lg:inline-flex" })}>
          <CalendarDays className="size-4" />
          New post
        </Link>
      </div>

      <div className="grid grid-cols-1 gap-6 2xl:grid-cols-[minmax(0,1fr)_320px]">
        {/* Calendar */}
        <Card>
          <CardHeader className="flex flex-wrap items-center justify-between gap-2 px-4 pb-3 sm:px-6">
            <CardTitle className="text-lg font-semibold">
              {MONTH_NAMES[viewMonth]} {viewYear}
            </CardTitle>
            <div className="flex items-center gap-1">
              <Button variant="ghost" size="icon" className="size-11 xl:size-8" onClick={prevMonth} aria-label="Previous month">
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="min-h-11 xl:min-h-0"
                onClick={() => {
                  setViewYear(today.getFullYear());
                  setViewMonth(today.getMonth());
                }}
              >
                Today
              </Button>
              <Button variant="ghost" size="icon" className="size-11 xl:size-8" onClick={nextMonth} aria-label="Next month">
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>
          {isCalendarError ? (
            <CardContent className="space-y-3 px-4 sm:px-6" role="alert">
              <p className="text-sm text-muted-foreground">Could not load this month.</p>
              <Button variant="outline" onClick={() => refetch()}>Try again</Button>
            </CardContent>
          ) : (
            <CardContent className="space-y-4 px-4 sm:px-6 xl:hidden">
              {isCalendarLoading ? (
                <p role="status" className="text-sm text-muted-foreground">Loading calendar…</p>
              ) : Object.keys(postsByDay).length === 0 ? (
                <p className="rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground">
                  No posts on the calendar this month.
                </p>
              ) : (
                Object.entries(postsByDay).map(([day, posts]) => (
                  <div key={day}>
                    <h3 className="mb-2 text-sm font-semibold">
                      {new Date(viewYear, viewMonth, Number(day)).toLocaleDateString(undefined, {
                        weekday: "long",
                        month: "short",
                        day: "numeric",
                      })}
                    </h3>
                    <ul className="divide-y rounded-lg border">
                      {posts.map((post) => (
                        <li key={post.id}>
                          <Link href={"/posts/" + post.id} className="block rounded-lg p-3 hover:bg-muted/50 focus-visible:outline-2">
                            <p className="line-clamp-2 wrap-anywhere text-sm font-medium">
                              {post.caption ?? post.title ?? "Untitled post"}
                            </p>
                            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                              <span>{new Date(post.scheduled_at ?? post.created_at).toLocaleTimeString(undefined, { timeStyle: "short" })}</span>
                              <StatusBadge status={post.status} />
                            </div>
                          </Link>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))
              )}
            </CardContent>
          )}

          {!isCalendarError && (
            <CardContent className="hidden xl:block">
              {/* Day headers */}
              <div className="mb-2 grid grid-cols-7 gap-1 text-center">
                {DAY_NAMES.map((d) => (
                  <div key={d} className="py-1 text-xs font-medium text-muted-foreground">
                    {d}
                  </div>
                ))}
              </div>

              {/* Calendar grid */}
              {isCalendarLoading ? (
                <div className="grid grid-cols-7 gap-1">
                  {Array.from({ length: 35 }).map((_, i) => (
                    <LoadingSkeleton key={i} className="h-[90px]" />
                  ))}
                </div>
              ) : (
                <div className="grid grid-cols-7 gap-1">
                  {Array.from({ length: firstDayOfWeek }).map((_, i) => (
                    <div key={`empty-start-${i}`} className="min-h-[90px]" />
                  ))}

                  {Array.from({ length: totalDays }).map((_, i) => {
                    const day = i + 1;
                    const cellDate = new Date(viewYear, viewMonth, day);
                    return (
                      <CalendarCell
                        key={day}
                        day={day}
                        year={viewYear}
                        month={viewMonth}
                        posts={postsByDay[day] ?? []}
                        isToday={isSameDay(cellDate, today)}
                        isCurrentMonth={true}
                      />
                    );
                  })}
                </div>
              )}
            </CardContent>
          )}
        </Card>

        {/* Upcoming sidebar */}
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base font-semibold">
                <Clock className="h-4 w-4 text-muted-foreground" />
                Upcoming
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 p-4 pt-0">
              {isUpcomingLoading ? (
                Array.from({ length: 4 }).map((_, i) => (
                  <LoadingSkeleton key={i} className="h-[72px]" />
                ))
              ) : upcomingPosts.length === 0 ? (
                <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                  <CalendarDays className="mx-auto mb-2 h-8 w-8 opacity-40" />
                  <p className="font-medium">No upcoming posts</p>
                  <p className="mt-1 text-xs">Schedule a post from the composer.</p>
                  <Link
                    href="/posts/new"
                    className="mt-2 inline-block text-xs text-primary hover:underline"
                  >
                    Create a post →
                  </Link>
                </div>
              ) : (
                upcomingPosts.map((post) => (
                  <UpcomingPostItem
                    key={post.id}
                    post={post}
                    onCancelSchedule={handleCancelSchedule}
                    onReschedule={setReschedulingPost}
                  />
                ))
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
