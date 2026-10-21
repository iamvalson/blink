"use client";

import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { postQueryKey, postsQueryKey } from "./use-posts";
import type { PostResponse } from "@/types/post";

const API_URL = process.env.NEXT_PUBLIC_API_URL;

/** Post statuses that will never change again. */
const TERMINAL_STATUSES = new Set([
  "PUBLISHED",
  "PARTIALLY_PUBLISHED",
  "FAILED",
  "CANCELLED",
]);

/**
 * usePostStatusStream subscribes to the server-sent event stream for a single
 * post and keeps the React Query cache up-to-date in real time.
 *
 * - Opens `GET /api/v1/posts/{id}/status` as an EventSource.
 * - On each `status` event the post query cache is updated directly so that any
 *   component using `usePost(id)` re-renders without an extra network round-trip.
 * - The list-posts and upcoming-posts caches are invalidated (not updated) so
 *   they refetch lazily on next focus or mount.
 * - The stream is closed and the hook becomes a no-op once a terminal status is
 *   received, or when the component unmounts.
 *
 * @param postId - UUID of the post to watch. Pass `null` to disable.
 * @param enabled - Opt-out flag (default: true).
 */
export function usePostStatusStream(
  postId: string | null,
  enabled = true,
): void {
  const queryClient = useQueryClient();
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!enabled || !postId || !API_URL) return;

    // Don't open a second stream if one is already running for this post.
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }

    const url = `${API_URL}/api/v1/posts/${postId}/status`;
    const es = new EventSource(url, { withCredentials: true });
    esRef.current = es;

    const handleStatus = (event: MessageEvent) => {
      try {
        const post: PostResponse = JSON.parse(event.data as string);

        // Update the individual post cache so detail views respond instantly.
        queryClient.setQueryData(postQueryKey(postId), post);

        // Invalidate list caches — they will refetch on next render.
        queryClient.invalidateQueries({ queryKey: postsQueryKey() });

        if (TERMINAL_STATUSES.has(post.status)) {
          es.close();
          esRef.current = null;
        }
      } catch {
        // Malformed JSON — ignore and wait for the next event.
      }
    };

    const handleDone = () => {
      es.close();
      esRef.current = null;
      // Final invalidation so the list shows the resolved state.
      queryClient.invalidateQueries({ queryKey: postsQueryKey() });
    };

    const handleError = () => {
      // EventSource will reconnect automatically unless we close explicitly.
      // Only close if the post is already in a terminal state in the cache.
      const cached = queryClient.getQueryData<PostResponse>(
        postQueryKey(postId),
      );
      if (cached && TERMINAL_STATUSES.has(cached.status)) {
        es.close();
        esRef.current = null;
      }
    };

    es.addEventListener("status", handleStatus);
    es.addEventListener("done", handleDone);
    es.addEventListener("error", handleError);

    return () => {
      es.removeEventListener("status", handleStatus);
      es.removeEventListener("done", handleDone);
      es.removeEventListener("error", handleError);
      es.close();
      esRef.current = null;
    };
  }, [postId, enabled, queryClient]);
}

