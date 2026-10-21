"use client";

import { useQuery } from "@tanstack/react-query";

import { getPost, listPosts, listCalendarPosts, listUpcomingPosts } from "../api/create-post";
import { usePostStatusStream } from "./use-post-status-stream";
import type { ListPostsFilter } from "@/types/post";

export const postsQueryKey = (filter?: ListPostsFilter) =>
  ["posts", filter] as const;

export const postQueryKey = (id: string) => ["posts", id] as const;

export function usePosts(filter: ListPostsFilter = {}) {
  return useQuery({
    queryKey: postsQueryKey(filter),
    queryFn: () => listPosts(filter),
  });
}

/**
 * usePost fetches a single post and subscribes to its real-time status stream
 * via SSE. Polling is used as a fallback when EventSource is unavailable (e.g.
 * in test environments or behind a proxy that doesn't support streaming).
 */
export function usePost(id: string | null, enabled = true) {
  const sseEnabled =
    enabled && id !== null && typeof EventSource !== "undefined";

  // Real-time updates via SSE — keeps the cache hot without polling.
  usePostStatusStream(id, sseEnabled);

  return useQuery({
    queryKey: postQueryKey(id ?? ""),
    queryFn: () => getPost(id!),
    enabled: enabled && id !== null,
    // Fall back to polling only when SSE is unavailable (non-browser or proxy
    // that drops EventSource connections).
    refetchInterval: sseEnabled
      ? false
      : (query) => {
          const data = query.state.data;
          if (!data) return false;
          const terminalStatuses = [
            "PUBLISHED",
            "PARTIALLY_PUBLISHED",
            "FAILED",
            "CANCELLED",
          ];
          if (terminalStatuses.includes(data.status)) return false;
          const allTargetsTerminal =
            !data.targets ||
            data.targets.length === 0 ||
            data.targets.every(
              (t) => t.status === "PUBLISHED" || t.status === "FAILED",
            );
          if (allTargetsTerminal && terminalStatuses.includes(data.status))
            return false;
          return 3000;
        },
    refetchIntervalInBackground: false,
  });
}

export const calendarQueryKey = (from: string, to: string, status?: string, platform?: string) =>
  ["calendar", from, to, status, platform] as const;

export function useCalendarPosts(
  from: Date,
  to: Date,
  status?: string,
  platform?: string,
) {
  return useQuery({
    queryKey: calendarQueryKey(from.toISOString(), to.toISOString(), status, platform),
    queryFn: () => listCalendarPosts(from, to, status, platform),
  });
}

export const upcomingQueryKey = (limit: number, offset: number) =>
  ["posts", "upcoming", limit, offset] as const;

export function useUpcomingPosts(limit = 20, offset = 0) {
  return useQuery({
    queryKey: upcomingQueryKey(limit, offset),
    queryFn: () => listUpcomingPosts({ limit, offset }),
    refetchInterval: 30_000,
  });
}
