import { Plus } from "lucide-react";
import Link from "next/link";

export default function DashboardPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-6 py-8">
      <div className="flex flex-row justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Overview</h1>

          <p className="mt-1 text-sm text-muted-foreground">
            Publishing activity across your workspace.
          </p>
        </div>
        <Link
          href="/posts/new"
          className="inline-flex items-center gap-3 rounded-md bg-black px-4 py-3 text-sm font-medium text-white transition-colors hover:bg-black/90"
        >
          <Plus className="size-4" />
          Create Post
        </Link>
      </div>
    </div>
  );
}
