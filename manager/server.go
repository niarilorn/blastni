// Copyright (c) 2019 Minoru Osuka
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manager

import (
	"log"

	accesslog "github.com/mash/go-accesslog"
	"github.com/mosuka/blast/grpc"
	"github.com/mosuka/blast/http"
)

type Server struct {
	id       string
	metadata map[string]interface{}

	peerAddr string

	indexConfig map[string]interface{}

	raftServer  *RaftServer
	grpcService *GRPCService
	grpcServer  *grpc.Server
	httpRouter  *http.Router
	httpServer  *http.Server

	logger     *log.Logger
	httpLogger accesslog.Logger
}

func NewServer(id string, metadata map[string]interface{}, peerAddr string, indexConfig map[string]interface{}, logger *log.Logger, httpLogger accesslog.Logger) (*Server, error) {
	return &Server{
		id:          id,
		metadata:    metadata,
		peerAddr:    peerAddr,
		indexConfig: indexConfig,
		logger:      logger,
		httpLogger:  httpLogger,
	}, nil
}

func (s *Server) Start() {
	var err error

	// bootstrap node?
	bootstrap := s.peerAddr == ""
	s.logger.Printf("[INFO] bootstrap: %v", bootstrap)

	// create raft server
	s.raftServer, err = NewRaftServer(s.id, s.metadata, bootstrap, s.indexConfig, s.logger)
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
		return
	}

	// create gRPC service
	s.grpcService, err = NewGRPCService(s.raftServer, s.logger)
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
		return
	}

	// create gRPC server
	s.grpcServer, err = grpc.NewServer(s.metadata["grpc_addr"].(string), s.grpcService, s.logger)
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
		return
	}

	// create HTTP router
	s.httpRouter, err = NewRouter(s.metadata["grpc_addr"].(string), s.logger)
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
		return
	}

	// create HTTP server
	s.httpServer, err = http.NewServer(s.metadata["http_addr"].(string), s.httpRouter, s.logger, s.httpLogger)
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
		return
	}

	// start Raft server
	s.logger.Print("[INFO] start Raft server")
	err = s.raftServer.Start()
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
		return
	}

	// start gRPC service
	s.logger.Print("[INFO] start gRPC service")
	go func() {
		err := s.grpcService.Start()
		if err != nil {
			s.logger.Printf("[ERR] %v", err)
			return
		}
	}()

	// start gRPC server
	s.logger.Print("[INFO] start gRPC server")
	go func() {
		err := s.grpcServer.Start()
		if err != nil {
			s.logger.Printf("[ERR] %v", err)
			return
		}
	}()

	// start HTTP server
	s.logger.Print("[INFO] start HTTP server")
	go func() {
		_ = s.httpServer.Start()
	}()

	// join to the existing cluster
	if !bootstrap {
		client, err := grpc.NewClient(s.peerAddr)
		defer func() {
			err := client.Close()
			if err != nil {
				s.logger.Printf("[ERR] %v", err)
			}
		}()
		if err != nil {
			s.logger.Printf("[ERR] %v", err)
			return
		}

		err = client.SetNode(s.id, s.metadata)
		if err != nil {
			s.logger.Printf("[ERR] %v", err)
			return
		}
	}
}

func (s *Server) Stop() {
	// stop HTTP server
	s.logger.Print("[INFO] stop HTTP server")
	err := s.httpServer.Stop()
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
	}

	// stop HTTP router
	err = s.httpRouter.Close()
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
	}

	// stop gRPC server
	s.logger.Print("[INFO] stop gRPC server")
	err = s.grpcServer.Stop()
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
	}

	// stop gRPC service
	s.logger.Print("[INFO] stop gRPC service")
	err = s.grpcService.Stop()
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
	}

	// stop Raft server
	s.logger.Print("[INFO] stop Raft server")
	err = s.raftServer.Stop()
	if err != nil {
		s.logger.Printf("[ERR] %v", err)
	}
}