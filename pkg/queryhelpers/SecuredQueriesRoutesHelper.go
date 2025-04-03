package queryhelpers

import (
	"errors"
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

type SecuredQueriesRoutesHelper struct {
	*serviceBase.ServiceBase
	store *implementations.BaseQueryStore
}

func NewSecuredQueriesRoutesHelper(queryService *serviceBase.ServiceBase, authModelIdentity *security.AuthModel, authModelNoIdentity *security.AuthModel) *SecuredQueriesRoutesHelper {
	path := queryService.Configuration.GetString("RESDIR_PATH")

	if _, err := os.Stat(path + models.QUERIES_FILE); errors.Is(err, os.ErrNotExist) {
		// file does not exist
		queryService.Logger.Fatalf("Public queries file <%s> does not exist. Shutting down.", path+models.QUERIES_FILE)
		return nil
	}

	store, err := implementations.NewPrivateQueryStore(queryService.Configuration, queryService.Logger)
	if err != nil {
		queryService.Logger.Fatalf("Failed to initialize PrivateQueryStore: %v", err)
		return nil
	}

	SecuredQueriesRoutesHelper := &SecuredQueriesRoutesHelper{
		ServiceBase: queryService,
		store:       store,
	}
	SecuredQueriesRoutesHelper.SetupRoutes(authModelIdentity, authModelNoIdentity)

	return SecuredQueriesRoutesHelper
}

func (s *SecuredQueriesRoutesHelper) SetupRoutes(authModelIdentity *security.AuthModel, authModelNoIdentity *security.AuthModel) {

	s.Logger.Infof("-----------------------------------------------")
	s.Logger.Infof("Secured routes (no identity) in this query service are:")

	var routeString = "/v1/queries/{serviceName}/{methodName}"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModelNoIdentity, s.handleNonIdentityRequiredQueries)

	s.Logger.Infof("-----------------------------------------------")
	s.Logger.Infof("Secured routes (with identity) in this query service are:")

	routeString = "/v1/identities/{identityId}/queries/{serviceName}/{methodName}"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModelIdentity, s.handleIdentityRequiredQueries)

}

func (s *SecuredQueriesRoutesHelper) handleIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()

	params := mux.Vars(r)
	queryParams := getQueryParams(r)

	// Add ownerId to query params because all user queries require it in their where clause
	queryParams["ownerId"] = params["identityId"]

	s.baseQueryHandler(w, r, queryParams)
}

func (s *SecuredQueriesRoutesHelper) handleNonIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()
	queryParams := getQueryParams(r)

	s.baseQueryHandler(w, r, queryParams)
}

func (s *SecuredQueriesRoutesHelper) baseQueryHandler(w http.ResponseWriter, r *http.Request, queryParams map[string]string) {
	params := mux.Vars(r)

	s.Logger.Infof("Incoming request to run the query: %s/%s", params["serviceName"], params["methodName"])

	jsonResults, err := s.store.RunStandAloneQuery(params["serviceName"], params["methodName"], queryParams)
	if err != nil {
		// TODO_PORT: log the error, but probably don't expose it to the client
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

/*
func (s *APIPrivateServer) writeHttpResponse(w http.ResponseWriter, status int, v []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(v)
}
*/

// TODO_PORT: This should probably be in a separate file (and package) for reuse by other controllers and services
func getQueryParams(r *http.Request) map[string]string {
	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}
	return queryParams
}

//func WriteJSON(w http.ResponseWriter, status int, v any) error {
//	w.Header().Add("Content-Type", "application/json")
//	w.WriteHeader(status)
//
//	return json.NewEncoder(w).Encode(v)
//}
