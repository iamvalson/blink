import { PostDetails } from "@/features/posts/components/post-details";

export const metadata = { title: "Post details | Blink" };

export default async function PostDetailsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <PostDetails id={id} />;
}
