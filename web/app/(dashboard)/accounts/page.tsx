import { AccountsList } from "@/features/accounts/components/accounts-list";

export default function AccountsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-6 py-8">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Accounts</h1>

        <p className="mt-1 text-muted-foreground">
          Connect your social accounts to publish through Blink.
        </p>
      </div>

      <AccountsList />
    </div>
  );
}
