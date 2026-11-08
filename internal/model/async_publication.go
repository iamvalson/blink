package model

import "time"

type AsyncPublication struct {
	SendToInbox  bool
	PublishID    *string
	RemoteStatus string
	PollCount    int
	NextCheckAt  *time.Time
	Deadline     time.Time
}
