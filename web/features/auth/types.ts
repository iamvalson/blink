export interface User {
  id: string;
  email: string;
  display_name: string;
}

export interface SignupInput {
  email: string;
  display_name: string;
  password: string;
  accept_terms: boolean;
  legal_version: string;
}

export interface SignupResponse {
  user_id: string;
}

export interface LoginInput {
  email: string;
  password: string;
  remember_me: boolean;
}

export interface LoginResponse {
  user_id: string;
}
