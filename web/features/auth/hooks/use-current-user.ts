"use client";

import { useQuery } from "@tanstack/react-query";
import { getCurrentUser } from "../api/me";

export const currentUserQueryKey = ["auth", "me"] as const;

export function useCurrentUser() {
  return useQuery({
    queryKey: currentUserQueryKey,
    queryFn: getCurrentUser,
    retry: false,
  });
}
