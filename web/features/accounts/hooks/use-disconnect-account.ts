"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { disconnectAccount } from "../api/disconnect-account";
import { accountsQueryKey } from "./use-accounts";

export function useDisconnectAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: disconnectAccount,

    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: accountsQueryKey,
      });
    },
  });
}
