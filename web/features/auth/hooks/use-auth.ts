"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { login } from "../api/login";
import { logout } from "../api/logout";
import { getCurrentUser } from "../api/me";
import { signup } from "../api/signup";
import type { LoginInput, SignupInput } from "../types";
import { currentUserQueryKey } from "./use-current-user";

export function useAuth() {
  const queryClient = useQueryClient();

  const signupMutation = useMutation({
    mutationFn: (input: SignupInput) => signup(input),
    onSuccess: async () => {
      await queryClient.query({
        queryKey: currentUserQueryKey,
        queryFn: getCurrentUser,
      });
    },
  });

  const loginMutation = useMutation({
    mutationFn: (input: LoginInput) => login(input),
    onSuccess: async () => {
      await queryClient.query({
        queryKey: currentUserQueryKey,
        queryFn: getCurrentUser,
      });
    },
  });

  const logoutMutation = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.removeQueries({
        queryKey: currentUserQueryKey,
      });
    },
  });

  return {
    signup: signupMutation,
    login: loginMutation,
    logout: logoutMutation,
  };
}
