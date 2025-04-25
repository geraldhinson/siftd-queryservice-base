package queryhelpers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/geraldhinson/siftd-base/pkg/constants"
	"github.com/geraldhinson/siftd-base/pkg/security"
	"github.com/geraldhinson/siftd-base/pkg/serviceBase"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/implementations"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/models"
)

type PublicQueriesRouter struct {
	*serviceBase.ServiceBase
	store *implementations.BaseQueryStore
}

func NewPublicQueriesRouter(
	service *serviceBase.ServiceBase,
	policyTranslation *models.QueryFileAuthPoliciesList) *PublicQueriesRouter {

	path := service.Configuration.GetString("RESDIR_PATH")

	if _, err := os.Stat(path + models.PUBLIC_QUERIES_FILE); errors.Is(err, os.ErrNotExist) {
		// file does not exist
		service.Logger.Fatalf("Public queries file <%s> does not exist. Shutting down.", path+models.PUBLIC_QUERIES_FILE)
		return nil
	}

	store, err := implementations.NewPublicQueryStore(service.Configuration, service.Logger)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize PublicQueryStore in query service: %v", err)
		return nil
	}

	queryMethod2AuthModel_Mapping := buildAuthModelsForQueries(service, store, policyTranslation)
	if queryMethod2AuthModel_Mapping == nil {
		service.Logger.Fatalf("Failed to build auth models for queries in query service: %v", err)
		return nil
	}

	publicQueriesRouter := &PublicQueriesRouter{
		ServiceBase: service,
		store:       store,
	}

	publicQueriesRouter.setupRoutes(queryMethod2AuthModel_Mapping)

	return publicQueriesRouter
}

func buildAuthModelsForQueries(
	service *serviceBase.ServiceBase,
	store *implementations.BaseQueryStore,
	policyTranslation *models.QueryFileAuthPoliciesList) map[int]*security.AuthModel {

	// loop through all methods in the query store and build the auth models
	// for each method, check if the authRequired string is in the policyTranslation map
	// if it is, use the corresponding auth model
	queryMethod2AuthModel_Mapping := make(map[int]*security.AuthModel)

	for methodIndex, method := range store.Methods {
		if method.Enabled == false {
			continue
		}

		var authModelInProgress *security.AuthModel
		var err error
		for _, authRequired := range method.AuthRequired {
			queryFilePolicy, ok := (*policyTranslation)[authRequired]
			if !ok {
				log.Printf("AuthRequired string <%s> was not found in the provided map of auth policy translations for method %d with query %s",
					authRequired, methodIndex, method.Query)
				return nil
			}

			if authModelInProgress == nil {
				authModelInProgress, err = service.NewAuthModel(
					queryFilePolicy.Realm,
					queryFilePolicy.AuthType,
					queryFilePolicy.Timeout,
					queryFilePolicy.ApprovedList,
				)
				if err != nil {
					// log the error and fail hard so forced to deal with
					log.Printf("Failed service.AuthModel() call in default PublicQueriesRouter for query service: %v", err)
					return nil
				}
			} else {
				err = authModelInProgress.AddPolicy(queryFilePolicy.Realm, queryFilePolicy.AuthType, queryFilePolicy.Timeout, queryFilePolicy.ApprovedList)
				if err != nil {
					log.Printf("Failed AuthModel.AddPolicy() call in default PublicQueriesRouter for query service: %v", err)
					return nil
				}
			}
		}
		// add the auth model to the queryMethod2AuthModel_Mapping
		queryMethod2AuthModel_Mapping[methodIndex] = authModelInProgress
		authModelInProgress = nil
	}
	return queryMethod2AuthModel_Mapping
}

func (s *PublicQueriesRouter) setupRoutes(method2AuthModelMap map[int]*security.AuthModel) {
	s.Logger.Infof("-----------------------------------------------")
	s.Logger.Infof("Unsecured (public) routes in this query service are:")

	// loop through all methods in the query store and build the auth models
	var routeString string
	for methodIndex, method := range s.store.Methods {
		if method.Enabled == false {
			continue
		}
		routeString = fmt.Sprintf("/v1/public/queries/%s/%s", method.ServiceName, method.MethodName)
		s.RegisterRoute(constants.HTTP_GET, routeString, method2AuthModelMap[methodIndex], s.handlePublicQueries)
	}

	// now register the non-database routes (TODO: move this to HealthCheckRouter)
	authModel, err := s.NewAuthModel(security.NO_REALM, security.NO_AUTH, security.NO_EXPIRY, nil)
	if err != nil {
		s.Logger.Fatalf("Failed to initialize AuthModel in default PublicQueriesRouter for query service: %v", err)
		return
	}

	routeString = "/v1/public/queries"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModel, s.handleGetQueryList)
}

func (s *PublicQueriesRouter) handleGetQueryList(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()

	s.Logger.Infof("Incoming request to get the list of defined queries")

	jsonResults, err := s.store.GetQueryList()
	if err != nil {
		// TODO_PORT: log the error, but probably don't expose it to the client
		s.Logger.Info("Failed to run query: ", err)

		writeHttpResponse(w, http.StatusBadRequest, []byte(err.Error()))
		return
	}

	writeHttpResponse(w, http.StatusOK, jsonResults)
}

func (s *PublicQueriesRouter) handlePublicQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()

	params := getURLPathParams("/v1/public/queries/", r)
	queryParams := getQueryParams(r)

	s.Logger.Infof("Incoming request to run the query: %s/%s", params["serviceName"], params["methodName"])

	jsonResults, err := s.store.RunStandAloneQuery(params["serviceName"], params["methodName"], queryParams)
	if err != nil {
		s.Logger.Info("Failed to run query: ", err)

		// check if err contains our constant indicating an internal server error and return 500 if it does
		if strings.Contains(err.Error(), models.INTERNAL_SERVER_ERROR) {
			writeHttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
		} else {
			writeHttpResponse(w, http.StatusBadRequest, []byte(err.Error()))
		}
		return
	}

	if models.DEBUGTRACE == true {
		s.Logger.Println("Result from RunStandAloneQuery() was: ", string(jsonResults))
	}

	writeHttpResponse(w, http.StatusOK, jsonResults)
}

func writeHttpResponse(w http.ResponseWriter, status int, v []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(v)
}
