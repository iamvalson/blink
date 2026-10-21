package jobs

import (
	"github.com/google/uuid"
	"testing"
)

func TestPublishJobPreservesTargetAndGeneration(t *testing.T) {
	target := uuid.New()
	want := PublishJob{PostID: uuid.New(), TargetID: &target, SchedulingGeneration: 7}
	task, err := NewPublishTaskForJob(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePublishJob(task.Payload())
	if err != nil {
		t.Fatal(err)
	}
	if got.PostID != want.PostID || got.TargetID == nil || *got.TargetID != target || got.SchedulingGeneration != 7 {
		t.Fatalf("round trip lost routing: %+v", got)
	}
}
func TestPublishJobRejectsMissingPostID(t *testing.T) {
	if _, err := NewPublishTask(uuid.Nil); err == nil {
		t.Fatal("empty post ID accepted")
	}
	if _, err := ParsePublishJob([]byte(`{}`)); err == nil {
		t.Fatal("missing post ID accepted")
	}
}
