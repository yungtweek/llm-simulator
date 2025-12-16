package grpc

import (
	"net"

	llmv1 "github.com/yungtweek/llm-simulator/gen"
	"github.com/yungtweek/llm-simulator/internal/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server wraps a gRPC server and its listen address.
//
// It is intentionally small/simple because this project is a benchmark/mock tool,
// not a production service framework.
type Server struct {
	addr       string
	grpcServer *grpc.Server
}

// NewGRPCServer creates a new gRPC server for the LlmService at the given address.
// Example addr: ":50051".
func NewGRPCServer(addr string, svc llmv1.LlmServiceServer) *Server {
	s := &Server{
		addr:       addr,
		grpcServer: grpc.NewServer(),
	}

	llmv1.RegisterLlmServiceServer(s.grpcServer, svc)
	// Handy during local development; harmless for a mock server.
	reflection.Register(s.grpcServer)

	return s
}

// Run starts listening on the configured address and serves the gRPC server.
// This call blocks until the server stops or returns an error.
func (s *Server) Run() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		logger.Log.Errorw("[grpc] failed to listen", "addr", s.addr, "err", err)
		return err
	}

	logger.Log.Infow("[grpc] starting server", "addr", s.addr)
	if err := s.grpcServer.Serve(lis); err != nil {
		logger.Log.Errorw("[grpc] server stopped with error", "err", err)
		return err
	}

	logger.Log.Info("[grpc] server stopped gracefully")
	return nil
}

// GracefulStop gracefully stops the underlying gRPC server.
func (s *Server) GracefulStop() {
	logger.Log.Infow("[grpc] graceful stop", "addr", s.addr)
	s.grpcServer.GracefulStop()
}

// Stop immediately stops the underlying gRPC server.
func (s *Server) Stop() {
	logger.Log.Infow("[grpc] stop", "addr", s.addr)
	s.grpcServer.Stop()
}
