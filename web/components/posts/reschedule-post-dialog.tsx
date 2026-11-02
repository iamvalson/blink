"use client";

import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { reschedulePost } from "@/features/posts/api/create-post";
import type { PostResponse } from "@/types/post";
import { ScheduleDialog } from "./schedule-dialog";

export function ReschedulePostDialog({
  post,
  onClose,
}: {
  post: PostResponse;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [isSaving, setIsSaving] = useState(false);
  const saving = useRef(false);

  const handleReschedule = async (scheduledAt: string) => {
    if (saving.current) return;
    saving.current = true;
    setIsSaving(true);
    try {
      await reschedulePost(post.id, { scheduled_at: scheduledAt });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["posts"] }),
        queryClient.invalidateQueries({ queryKey: ["calendar"] }),
      ]);
      toast.success("Post rescheduled.");
      onClose();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not reschedule the post.");
      // A worker may have claimed the post while this dialog was open.
      void queryClient.invalidateQueries({ queryKey: ["posts"] });
      void queryClient.invalidateQueries({ queryKey: ["calendar"] });
    } finally {
      saving.current = false;
      setIsSaving(false);
    }
  };

  return (
    <ScheduleDialog
      open
      mode="reschedule"
      initialScheduledAt={post.scheduled_at}
      isSubmitting={isSaving}
      onOpenChange={(open) => { if (!open) onClose(); }}
      onSchedule={handleReschedule}
    />
  );
}
