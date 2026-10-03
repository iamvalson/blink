package worker

import (
	"time"

	"github.com/hibiken/asynq"
)

type Server struct {
	server *asynq.Server
}

func NewServer(redisAddr string) (*Server, error) {
	return NewServerWithShutdownTimeout(redisAddr, 30*time.Second), nil
}

func NewServerWithShutdownTimeout(redisAddr string, shutdownTimeout time.Duration) *Server {
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"default": 10,
			},
			LogLevel:        asynq.InfoLevel,
			ShutdownTimeout: shutdownTimeout,
		},
	)

	return &Server{server: srv}
}

func (s *Server) Start(mux *asynq.ServeMux) error {
	return s.server.Run(mux)
}

func (s *Server) Shutdown() {
	s.server.Shutdown()
}
