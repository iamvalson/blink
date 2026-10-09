"use client";

import { CalendarClock } from "lucide-react";
import { useState } from "react";

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
  onSchedule: (date: string) => void;
};

export function ScheduleDialog({
  open,
  onOpenChange,
  onSchedule,
}: ScheduleDialogProps) {
  const [date, setDate] = useState("");

  const handleSchedule = () => {
    if (!date) return;

    onSchedule(date);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-full bg-muted">
            <CalendarClock className="h-5 w-5" />
          </div>

          <DialogTitle>Schedule post</DialogTitle>

          <DialogDescription>
            Choose when Blink should publish this post to your selected
            platforms.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2 py-4">
          <Label htmlFor="schedule-date">Date and time</Label>

          <Input
            id="schedule-date"
            type="datetime-local"
            value={date}
            onChange={(event) => setDate(event.target.value)}
            min={new Date().toISOString().slice(0, 16)}
          />
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>

          <Button type="button" disabled={!date} onClick={handleSchedule}>
            Schedule post
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
