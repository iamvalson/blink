"use client";

import { useQuery } from "@tanstack/react-query";

import { getPost, listPosts, listCalendarPosts, listUpcomingPosts } from "../api/create-post";
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

export function usePost(id: string | null, enabled = true) {
  return useQuery({
    queryKey: postQueryKey(id ?? ""),
    queryFn: () => getPost(id!),
    enabled: enabled && id !== null,
    // Poll while the post is in a non-terminal state
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      const terminalStatuses = ["PUBLISHED", "FAILED"];
      if (terminalStatuses.includes(data.status)) return false;
      // Check individual targets too
      const allTargetsTerminal =
        !data.targets ||
        data.targets.length === 0 ||
        data.targets.every(
          (t) => t.status === "PUBLISHED" || t.status === "FAILED",
        );
      if (allTargetsTerminal && terminalStatuses.includes(data.status))
        return false;
      // Poll every 3 seconds while active
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
