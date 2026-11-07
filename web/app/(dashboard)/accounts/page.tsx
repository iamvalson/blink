import { AccountsList } from "@/features/accounts/components/accounts-list";

export default function AccountsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
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
