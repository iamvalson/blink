package model

import "time"

type AsyncPublication struct {
	PublishID    *string
	RemoteStatus string
	PollCount    int
	NextCheckAt  *time.Time
	Deadline     time.Time
}
