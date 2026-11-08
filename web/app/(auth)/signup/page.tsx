import { GuestGuard } from "@/features/auth/components/guest-guard";
import { SignupForm } from "@/features/auth/components/signup-form";

export default function SignupPage() {
  return <GuestGuard><SignupForm /></GuestGuard>;
}
