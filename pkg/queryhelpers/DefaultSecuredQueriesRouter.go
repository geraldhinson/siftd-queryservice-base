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

type SecuredQueriesRouter struct {
	*serviceBase.ServiceBase
	store *implementations.BaseQueryStore
}

func NewSecuredQueriesRouter(
	service *serviceBase.ServiceBase,
	noIdentityProvidedRealm string,
	noIdentityProvidedAuthType security.AuthTypes,
	noIdentityProvidedTimeout security.AuthTimeout,
	noIdentityProvidedApproved []string,
	identityProvidedRealm string,
	identityProvidedAuthType security.AuthTypes,
	identityProvidedTimeout security.AuthTimeout,
	identityProvidedApproved []string) *SecuredQueriesRouter {
	path := service.Configuration.GetString("RESDIR_PATH")

	if _, err := os.Stat(path + models.QUERIES_FILE); errors.Is(err, os.ErrNotExist) {
		// file does not exist
		service.Logger.Fatalf("Public queries file <%s> does not exist. Shutting down.", path+models.QUERIES_FILE)
		return nil
	}

	store, err := implementations.NewPrivateQueryStore(service.Configuration, service.Logger)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize PrivateQueryStore: %v", err)
		return nil
	}

	authModelIdentity, err := service.NewAuthModel(noIdentityProvidedRealm, noIdentityProvidedAuthType, noIdentityProvidedTimeout, noIdentityProvidedApproved)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize AuthModel in default PublicQueriesRouter for query service: %v", err)
		return nil
	}

	authModelNoIdentity, err := service.NewAuthModel(identityProvidedRealm, identityProvidedAuthType, identityProvidedTimeout, identityProvidedApproved)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize AuthModel in default PublicQueriesRouter for query service: %v", err)
		return nil
	}

	securedQueriesRouter := &SecuredQueriesRouter{
		ServiceBase: service,
		store:       store,
	}
	securedQueriesRouter.setupRoutes(authModelIdentity, authModelNoIdentity)

	return securedQueriesRouter
}

func (s *SecuredQueriesRouter) setupRoutes(authModelIdentity *security.AuthModel, authModelNoIdentity *security.AuthModel) {

	s.Logger.Infof("-----------------------------------------------")
	s.Logger.Infof("Secured routes (no identity) in this query service are:")

	var routeString = "/v1/queries/{serviceName}/{methodName}"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModelNoIdentity, s.handleNonIdentityRequiredQueries)

	s.Logger.Infof("-----------------------------------------------")
	s.Logger.Infof("Secured routes (with identity) in this query service are:")

	routeString = "/v1/identities/{identityId}/queries/{serviceName}/{methodName}"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModelIdentity, s.handleIdentityRequiredQueries)

}

func (s *SecuredQueriesRouter) handleIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
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

func (s *SecuredQueriesRouter) handleNonIdentityRequiredQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()
	queryParams := getQueryParams(r)

	s.baseQueryHandler(w, r, queryParams)
}

func (s *SecuredQueriesRouter) baseQueryHandler(w http.ResponseWriter, r *http.Request, queryParams map[string]string) {
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

//func WriteJSON(w http.ResponseWriter, status int, v any) error {
//	w.Header().Add("Content-Type", "application/json")
//	w.WriteHeader(status)
//
//	return json.NewEncoder(w).Encode(v)
//}
