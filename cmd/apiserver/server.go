package main

import (
	"fantom-api-graphql/cmd/apiserver/build"
	"fantom-api-graphql/internal/config"
	"fantom-api-graphql/internal/graphql/resolvers"
	"fantom-api-graphql/internal/handlers"
	"fantom-api-graphql/internal/logger"
	"fantom-api-graphql/internal/repository"
	"fantom-api-graphql/internal/validator"
	"flag"
	"log"
	"net/http"
)

// ApiServer represents the server structure.
type ApiServer struct {
	cfg  *config.Config
	log  logger.Logger
	repo repository.Repository
	rv   resolvers.ApiResolver
	cv   *validator.ContractValidator

	// isVR indicates if this is just a version request
	isVersionReq *bool
}

// NewApiServer creates a new instance of the API server.
func NewApiServer(cfg *config.Config) (*ApiServer, error) {
	// make logger
	lg := logger.New(cfg)

	// create repository for data exchange with the blockchain full node and local persistent storage
	repo, err := repository.New(cfg, lg)
	if err != nil {
		return nil, err
	}

	// return the API server instance
	return &ApiServer{
		cfg:          cfg,
		log:          lg,
		repo:         repo,
		rv:           resolver(cfg, lg, repo),
		cv:           validator.NewContractValidator(cfg, repo, lg),
		isVersionReq: flag.Bool("v", false, "get the application version"),
	}, nil
}

// Run starts the API server.
func (api *ApiServer) Run() {
	// always print the version
	build.PrintVersion(api.cfg)

	// is this a simple version print? if so, simply terminate here
	if *api.isVersionReq {
		return
	}

	// start listening for incoming HTTP requests
	log.Fatal(http.ListenAndServe(api.cfg.Server.BindAddress, nil))
}

// Stop terminates the API server.
func (api *ApiServer) Stop() {
	// log
	api.log.Notice("API server is terminating")

	// signal close to modules
	cv.Close()
	repo.Close()
	rs.Close()

	// log we are done here
	api.log.Notice("API server closed")
}

// resolver builds and initializes the resolver
func resolver(cfg *config.Config, log logger.Logger, repo repository.Repository) resolvers.ApiResolver {
	// create root resolver
	rs := resolvers.New(cfg, log, repo)
	log.Notice("initialized, going live")

	// setup GraphQL API handler
	h := handlers.Api(cfg, log, rs)
	http.Handle("/api", h)
	http.Handle("/graphql", h)

	// handle GraphiQL interface
	http.Handle("/graphi", handlers.GraphiHandler(cfg.Server.DomainAddress, log))

	// log the server opening info
	log.Infof("welcome to Fantom GraphQL API server network interface.")
	log.Infof("listening for requests on [%s]", cfg.Server.BindAddress)

	return rs
}
