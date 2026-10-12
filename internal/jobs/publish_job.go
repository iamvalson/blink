package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type PublishJob struct {
	PostID               uuid.UUID `json:"post_id"`
	SchedulingGeneration int       `json:"scheduling_generation,omitempty"`
}


func NewPublishTask(postID uuid.UUID, schedulingGenerations ...int) (*asynq.Task, error) {
	gen := 0
	if len(schedulingGenerations) > 0 {
		gen = schedulingGenerations[0]
	}
	payload, err := json.Marshal(PublishJob{PostID: postID, SchedulingGeneration: gen})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypePublishPost, payload), nil
}


func ParsePublishJob(payload []byte) (*PublishJob, error) {
	var job PublishJob
	if err := json.Unmarshal(payload, &job); err != nil{
		return nil, err
	}
	return &job, nil
}