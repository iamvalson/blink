"use client";

import { CalendarClock, Clock } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type ScheduleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with an ISO-8601 UTC string when the user confirms */
  onSchedule: (isoDate: string) => void;
  isSubmitting?: boolean;
};

export function ScheduleDialog({
  open,
  onOpenChange,
  onSchedule,
  isSubmitting = false,
}: ScheduleDialogProps) {
  const [date, setDate] = useState("");

  // Minimum selectable datetime is "now + 5 min" to give backend wiggle-room
  const minDateTime = useMemo(() => {
    const d = new Date(Date.now() + 5 * 60 * 1000);
    // Format as YYYY-MM-DDTHH:mm (local time, required by datetime-local input)
    const pad = (n: number) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }, []);

  const selectedDate = date ? new Date(date) : null;
  const isValid =
    selectedDate !== null && selectedDate > new Date(Date.now() + 60 * 1000);

  const handleSchedule = () => {
    if (!isValid || !selectedDate) return;
    onSchedule(selectedDate.toISOString());
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
            <CalendarClock className="h-5 w-5 text-primary" />
          </div>

          <DialogTitle>Schedule post</DialogTitle>

          <DialogDescription>
            Choose when Blink should publish this post to your selected
            platforms. Times are in your local timezone.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="schedule-date">Date and time</Label>

            <Input
              id="schedule-date"
              type="datetime-local"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              min={minDateTime}
              className="w-full"
            />
          </div>

          {selectedDate && isValid && (
            <div className="flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-3 py-2 text-sm text-primary">
              <Clock className="h-4 w-4 shrink-0" />
              <span>
                Will publish{" "}
                <strong>
                  {selectedDate.toLocaleString(undefined, {
                    dateStyle: "medium",
                    timeStyle: "short",
                  })}
                </strong>
              </span>
            </div>
          )}

          {date && !isValid && (
            <p className="text-sm text-destructive">
              Please choose a time at least 1 minute in the future.
            </p>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            Cancel
          </Button>

          <Button
            type="button"
            disabled={!isValid || isSubmitting}
            onClick={handleSchedule}
          >
            {isSubmitting ? "Scheduling…" : "Schedule post"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
