package jobs

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type PublishJob struct {
	PostID               uuid.UUID  `json:"post_id"`
	TargetID             *uuid.UUID `json:"target_id,omitempty"`
	SchedulingGeneration int        `json:"scheduling_generation,omitempty"`
}

func NewPublishTask(postID uuid.UUID, generations ...int) (*asynq.Task, error) {
	generation := 0
	if len(generations) > 0 {
		generation = generations[0]
	}

	return NewPublishTaskForJob(PublishJob{
		PostID:               postID,
		SchedulingGeneration: generation,
	})
}

func NewPublishTaskForJob(job PublishJob) (*asynq.Task, error) {
	if job.PostID == uuid.Nil {
		return nil, errors.New("post ID is required")
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypePublishPost, payload), nil
}

func ParsePublishJob(payload []byte) (*PublishJob, error) {
	var job PublishJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return nil, err
	}
	if job.PostID == uuid.Nil {
		return nil, errors.New("post ID is required")
	}
	return &job, nil
}
