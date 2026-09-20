package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type PublishJob struct {
	PostID uuid.UUID `json:"post_id"`
}


func NewPublishTask(postID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(PublishJob{PostID: postID})
	if err != nil{
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