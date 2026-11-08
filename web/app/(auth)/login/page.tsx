import { GuestGuard } from "@/features/auth/components/guest-guard";
import { LoginForm } from "@/features/auth/components/login-form";

export default function LoginPage() {
  return <GuestGuard><LoginForm /></GuestGuard>;
}
