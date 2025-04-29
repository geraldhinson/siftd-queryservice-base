package queryhelpers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	//	"github.com/geraldhinson/siftd-base/pkg/constants"
	"github.com/geraldhinson/siftd-base/pkg/security"
	"github.com/geraldhinson/siftd-base/pkg/serviceBase"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/constants"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/implementations"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/models"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type SecuredQueriesRouter struct {
	*serviceBase.ServiceBase
	store      *implementations.BaseQueryStore
	debugLevel int
}

func NewSecuredQueriesRouter(
	service *serviceBase.ServiceBase,
	policyTranslation *models.QueryFileAuthPoliciesList) *SecuredQueriesRouter {

	var debugLevel = 0
	if service.Configuration.GetString(constants.DEBUGSIFTD_QUERYHELPERS) != "" {
		debugLevel = service.Configuration.GetInt(constants.DEBUGSIFTD_QUERYHELPERS)
	}

	path := service.Configuration.GetString("RESDIR_PATH")

	if _, err := os.Stat(path + constants.QUERIES_FILE); errors.Is(err, os.ErrNotExist) {
		// file does not exist
		service.Logger.Errorf("queryservice secured queries router - the secured queries file <%s> does not exist. Shutting down.", path+constants.QUERIES_FILE)
		return nil
	}

	store, err := implementations.NewPrivateQueryStore(service.Configuration, service.Logger)
	if err != nil {
		service.Logger.Errorf("queryservice secured queries router - failed to initialize the query store with: %v", err)
		return nil
	}

	queryMethod2AuthModel_Mapping, err := buildAuthModelsForQueries(service, store, policyTranslation)
	if err != nil {
		service.Logger.Errorf("queryservice secured queries router - failed to build auth models for secured queries with: %v", err)
		return nil
	}

	securedQueriesRouter := &SecuredQueriesRouter{
		ServiceBase: service,
		store:       store,
		debugLevel:  debugLevel,
	}

	err = securedQueriesRouter.setupRoutes(queryMethod2AuthModel_Mapping)
	if err != nil {
		service.Logger.Errorf("queryservice secured queries router - failed to setup routes for secured queries with: %v", err)
		return nil
	}

	return securedQueriesRouter
}

// TODO: make return an error vs calling Fatalf
func (s *SecuredQueriesRouter) setupRoutes(method2AuthModelMap map[int]*security.AuthModel) error {

	// loop through all methods in the query store and build the auth models
	var routeString string
	for methodIndex, method := range s.store.Methods {
		if method.Enabled == false {
			continue
		}
		if strings.Contains(method.ExampleCall, "/identities/") {
			// this is a query that requires an identity
			routeString = fmt.Sprintf("/v1/identities/{identityId}/queries/%s/%s", method.ServiceName, method.MethodName)
			s.RegisterRoute(constants.HTTP_GET, routeString, method2AuthModelMap[methodIndex], s.handleIdentityRequiredQueries)
		} else {
			routeString = fmt.Sprintf("/v1/queries/%s/%s", method.ServiceName, method.MethodName)
			s.RegisterRoute(constants.HTTP_GET, routeString, method2AuthModelMap[methodIndex], s.handleNonIdentityRequiredQueries)
		}
	}

	// now register the non-database routes (TODO: move this to HealthCheckRouter and maybe rename that to Mgmt..?)
	authModel, err := s.NewAuthModel(security.NO_REALM, security.NO_AUTH, security.NO_EXPIRY, nil)
	if err != nil {
		return fmt.Errorf("queryservice secured queries router - failed to initialize AuthModel in default PublicQueriesRouter for query service: %v", err)
	}

	routeString = "/v1/queries"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModel, s.handleGetQueryList)

	return nil
}

func (s *SecuredQueriesRouter) handleGetQueryList(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("queryservice secured queries router - Elapsed time for request: %v", elapsed)
	}()

	if s.debugLevel > 0 {
		s.Logger.Infof("queryservice secured queries router - incoming request to get the list of defined queries")
	}

	jsonResults, err := s.store.GetQueryList()
	if err != nil {
		s.Logger.Info("queryservice secured queries router - Failed to run query: ", err)

		writeHttpResponse(w, http.StatusBadRequest, []byte(err.Error()))
		return
	}

	writeHttpResponse(w, http.StatusOK, jsonResults)
}

func (s *SecuredQueriesRouter) handleIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("queryservice secured queries router - Elapsed time for request: %v", elapsed)
	}()

	if s.debugLevel > 0 {
		s.Logger.Infof("queryservice secured queries router - incoming identity-required query request: %s", r.URL.Path)
	}

	//	params := mux.Vars(r)
	urlParams := getURLPathParams(s.Logger, "/queries/", r)
	if urlParams == nil {
		s.Logger.Infof("queryservice secured queries router - Invalid URL path detected on incoming request - unable to find prefix in path: %s\n", r.URL.Path)
		writeHttpResponse(w, http.StatusBadRequest, []byte("Invalid URL path detected on incoming request - unable to find prefix in path"))
		return
	}
	queryParams := s.GetQueryParams(r)

	// Add ownerId to query params because all user queries require it in their where clause
	queryParams["ownerId"] = urlParams["identityId"]

	s.baseQueryHandler(w, r, urlParams, queryParams)
}

func (s *SecuredQueriesRouter) handleNonIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("queryservice secured queries router - Elapsed time for request: %v", elapsed)
	}()

	if s.debugLevel > 0 {
		s.Logger.Infof("queryservice secured queries router - incoming no-identity-required query request: %s", r.URL.Path)
	}

	urlParams := getURLPathParams(s.Logger, "/queries/", r)
	// TODO: check if urlParams is nil and causes a problem if so
	queryParams := s.GetQueryParams(r)

	s.baseQueryHandler(w, r, urlParams, queryParams)
}

func (s *SecuredQueriesRouter) baseQueryHandler(w http.ResponseWriter, r *http.Request, urlParams map[string]string, queryParams map[string]string) {

	jsonResults, err := s.store.RunStandAloneQuery(urlParams["serviceName"], urlParams["methodName"], queryParams)
	if err != nil {
		s.Logger.Info("queryservice secured queries router - Failed to run query: ", err)

		// check if err contains our constant indicating an internal server error and return 500 if it does
		if strings.Contains(err.Error(), constants.INTERNAL_SERVER_ERROR) {
			writeHttpResponse(w, http.StatusInternalServerError, []byte(err.Error()))
		} else {
			writeHttpResponse(w, http.StatusBadRequest, []byte(err.Error()))
		}
		return
	}

	if s.debugLevel > 1 {
		s.Logger.Println("queryservice secured queries router - the result from RunStandAloneQuery() was: ", string(jsonResults))
	}

	writeHttpResponse(w, http.StatusOK, jsonResults)
}

func getURLPathParams(logger *logrus.Logger, pathContains string, r *http.Request) map[string]string {
	params := make(map[string]string)
	// return suffix after contained string

	if strings.Contains(r.URL.Path, pathContains) {
		urlSuffix := strings.Split(r.URL.Path, pathContains)
		if len(urlSuffix) != 2 {
			logger.Infof("queryservice queries router - Invalid URL path detected on incoming request - unable to find suffix in path: %s\n", r.URL.Path)
			return nil
		}

		pathParts := strings.Split(urlSuffix[1], "/")
		if len(pathParts) != 2 {
			logger.Infof("queryservice queries router - Invalid URL path detected on incoming request - unable to find both service and method in path: %s\n", r.URL.Path)
			return nil
		}
		params["serviceName"] = pathParts[0]
		params["methodName"] = pathParts[1]

		requestParams := mux.Vars(r)
		if requestParams != nil && requestParams["identityId"] != "" {
			params["identityId"] = requestParams["identityId"]
		}

		return params
	} else {
		logger.Infof("queryservice queries router - Invalid URL path detected on incoming request - unable to find prefix in path: %s\n", r.URL.Path)
		return nil
	}
}
