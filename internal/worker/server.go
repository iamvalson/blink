package worker

import (
	"github.com/hibiken/asynq"
)

type Server struct {
	server *asynq.Server
}

func NewServer(redisAddr string) (*Server, error) {
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"default": 10,
			},
			LogLevel: asynq.InfoLevel,
		},
	)

	return &Server{server: srv}, nil
}

func (s *Server) Start(mux *asynq.ServeMux) error {
	return s.server.Run(mux)
}

func (s *Server) Shutdown() {
	s.server.Shutdown()
}