import { PostComposer } from "@/components/posts/post-composer";

export default function NewPostPage() {
  return (
    <main className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
      <div className="mb-8">
        <div className="flex items-center gap-3">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">
              Create a post
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Create, preview, and publish content across your connected
              platforms.
            </p>
          </div>
        </div>
      </div>

      <PostComposer />
    </main>
  );
}
