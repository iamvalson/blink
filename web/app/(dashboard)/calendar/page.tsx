"use client";

import { useCalendarPosts, useUpcomingPosts } from "@/features/posts/hooks/use-posts";
import { cancelSchedule } from "@/features/posts/api/create-post";
import { StatusBadge } from "@/components/posts/status-badge";
import type { PostResponse } from "@/types/post";
import {
  CalendarDays,
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

import { Button } from "@/components/ui/button";
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
}: {
  post: PostResponse;
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
        <p className="truncate text-sm font-medium leading-snug">
          {post.caption ?? post.title ?? "(no caption)"}
        </p>
        <div className="mt-1.5 flex flex-wrap items-center gap-2">
          {scheduledAt && (
            <span className="text-xs text-muted-foreground">
              {scheduledAt.toLocaleTimeString(undefined, { timeStyle: "short" })}
            </span>
          )}
          <div className="flex gap-1">
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
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100"
        >
          <MoreHorizontal className="h-4 w-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
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
        "relative flex min-h-[90px] flex-col rounded-lg p-2 text-sm transition-colors",
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
            <div
              key={post.id}
              title={post.caption ?? post.title ?? "(no caption)"}
              className={[
                "flex items-center gap-1 truncate rounded px-1 py-0.5 text-[11px] font-medium leading-tight",
                post.status === "SCHEDULED"
                  ? "bg-primary/10 text-primary"
                  : "bg-muted text-muted-foreground",
              ].join(" ")}
            >
              <span className="flex gap-0.5 text-[9px] uppercase tracking-tight">
                {platforms.join("/")}
              </span>
              <span className="truncate">
                {post.caption ?? post.title ?? "(no caption)"}
              </span>
            </div>
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

  const rangeFrom = useMemo(() => startOfMonth(viewYear, viewMonth), [viewYear, viewMonth]);
  const rangeTo = useMemo(() => endOfMonth(viewYear, viewMonth), [viewYear, viewMonth]);

  const { data: calendarPosts = [], isLoading: isCalendarLoading } =
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
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Content Calendar</h1>
          <p className="text-sm text-muted-foreground">
            View and manage your scheduled posts.
          </p>
        </div>
        <Button>
          <Link href="/posts" className="flex items-center gap-2">
            <CalendarDays className="h-4 w-4" />
            New post
          </Link>
        </Button>
      </div>

      <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
        {/* Calendar */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-3">
            <CardTitle className="text-lg font-semibold">
              {MONTH_NAMES[viewMonth]} {viewYear}
            </CardTitle>
            <div className="flex items-center gap-1">
              <Button variant="ghost" size="icon" onClick={prevMonth} aria-label="Previous month">
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setViewYear(today.getFullYear());
                  setViewMonth(today.getMonth());
                }}
              >
                Today
              </Button>
              <Button variant="ghost" size="icon" onClick={nextMonth} aria-label="Next month">
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </CardHeader>
          <CardContent>
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
                    href="/posts"
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
