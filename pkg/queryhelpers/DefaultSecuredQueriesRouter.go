package queryhelpers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/geraldhinson/siftd-base/pkg/constants"
	"github.com/geraldhinson/siftd-base/pkg/security"
	"github.com/geraldhinson/siftd-base/pkg/serviceBase"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/implementations"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/models"
	"github.com/gorilla/mux"
)

type SecuredQueriesRouter struct {
	*serviceBase.ServiceBase
	store *implementations.BaseQueryStore
}

func NewSecuredQueriesRouter(
	service *serviceBase.ServiceBase,
	policyTranslation *models.QueryFileAuthPoliciesList) *SecuredQueriesRouter {

	path := service.Configuration.GetString("RESDIR_PATH")

	if _, err := os.Stat(path + models.QUERIES_FILE); errors.Is(err, os.ErrNotExist) {
		// file does not exist
		service.Logger.Fatalf("Queries file <%s> does not exist. Shutting down.", path+models.QUERIES_FILE)
		return nil
	}

	store, err := implementations.NewPrivateQueryStore(service.Configuration, service.Logger)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize PrivateQueryStore: %v", err)
		return nil
	}

	queryMethod2AuthModel_Mapping := buildAuthModelsForQueries(service, store, policyTranslation)
	if queryMethod2AuthModel_Mapping == nil {
		service.Logger.Fatalf("Failed to build auth models for queries in query service: %v", err)
		return nil
	}

	securedQueriesRouter := &SecuredQueriesRouter{
		ServiceBase: service,
		store:       store,
	}
	securedQueriesRouter.setupRoutes(queryMethod2AuthModel_Mapping)

	return securedQueriesRouter
}

func (s *SecuredQueriesRouter) setupRoutes(method2AuthModelMap map[int]*security.AuthModel) {

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

	// now register the non-database routes (TODO: move this to HealthCheckRouter)
	authModel, err := s.NewAuthModel(security.NO_REALM, security.NO_AUTH, security.NO_EXPIRY, nil)
	if err != nil {
		s.Logger.Fatalf("Failed to initialize AuthModel in default PublicQueriesRouter for query service: %v", err)
		return
	}

	routeString = "/v1/queries"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModel, s.handleGetQueryList)

}

func (s *SecuredQueriesRouter) handleGetQueryList(w http.ResponseWriter, r *http.Request) {
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

func (s *SecuredQueriesRouter) handleIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()

	//	params := mux.Vars(r)
	urlParams := getURLPathParams("/queries/", r)
	queryParams := getQueryParams(r)

	// Add ownerId to query params because all user queries require it in their where clause
	queryParams["ownerId"] = urlParams["identityId"]

	s.baseQueryHandler(w, r, urlParams, queryParams)
}

func (s *SecuredQueriesRouter) handleNonIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()

	urlParams := getURLPathParams("/queries/", r)
	queryParams := getQueryParams(r)

	s.baseQueryHandler(w, r, urlParams, queryParams)
}

func (s *SecuredQueriesRouter) baseQueryHandler(w http.ResponseWriter, r *http.Request, urlParams map[string]string, queryParams map[string]string) {
	//	params := mux.Vars(r)

	s.Logger.Infof("Incoming request to run the query: %s/%s", urlParams["serviceName"], urlParams["methodName"])

	jsonResults, err := s.store.RunStandAloneQuery(urlParams["serviceName"], urlParams["methodName"], queryParams)
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
func getURLPathParams(pathContains string, r *http.Request) map[string]string {
	params := make(map[string]string)
	// return suffix after contained string

	if strings.Contains(r.URL.Path, pathContains) {
		urlSuffix := strings.Split(r.URL.Path, pathContains)
		if len(urlSuffix) != 2 {
			fmt.Printf("Invalid URL path detected on incoming request - unable to find suffix in path: %s\n", r.URL.Path)
			return nil
		}

		pathParts := strings.Split(urlSuffix[1], "/")
		if len(pathParts) != 2 {
			fmt.Printf("Invalid URL path detected on incoming request - unable to find both service and method in path: %s\n", r.URL.Path)
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
		fmt.Printf("Invalid URL path detected on incoming request - unable to find prefix in path: %s\n", r.URL.Path)
		return nil
	}
}

// TODO_PORT: (didn't I move this already? Making note to check.)This should probably be in a separate file (and package) for reuse by other controllers and services
func getQueryParams(r *http.Request) map[string]string {
	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}
	return queryParams
}
