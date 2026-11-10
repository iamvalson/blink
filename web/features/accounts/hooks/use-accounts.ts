"use client";

import { useQuery } from "@tanstack/react-query";

import { getAccounts } from "../api/get-accounts";

export const accountsQueryKey = ["accounts"];

export function useAccounts() {
  return useQuery({
    queryKey: accountsQueryKey,
    queryFn: getAccounts,
  });
}
