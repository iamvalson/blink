import { Plus } from "lucide-react";
import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { PostsList } from "@/features/posts/components/posts-list";

export const metadata = {
  title: "Posts | Blink",
  description: "Manage and monitor your published and scheduled posts.",
};

export default async function PostsPage({ searchParams }: { searchParams: Promise<{ status?: string }> }) {
  const { status } = await searchParams;
  const initialStatus = ["SCHEDULED", "QUEUED", "PUBLISHING", "PUBLISHED", "FAILED", "PARTIALLY_PUBLISHED"].includes(status ?? "") ? status : undefined;
  return (
    <div className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-4 sm:mb-8">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Posts</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage your posts and view live publishing status across connected platforms.
          </p>
        </div>
        <Link href="/posts/new" className={buttonVariants({ className: "hidden lg:inline-flex" })}>
          <Plus className="mr-2 h-4 w-4" />
          Create post
        </Link>
      </div>

      <PostsList key={initialStatus ?? "all"} initialStatus={initialStatus} />
    </div>
  );
}
