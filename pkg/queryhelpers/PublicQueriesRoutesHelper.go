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

type PublicQueriesRoutesHelper struct {
	*serviceBase.ServiceBase
	store *implementations.BaseQueryStore
}

func NewPublicQueriesRoutesHelper(queryService *serviceBase.ServiceBase, authModel *security.AuthModel) *PublicQueriesRoutesHelper {
	path := queryService.Configuration.GetString("RESDIR_PATH")

	if _, err := os.Stat(path + models.PUBLIC_QUERIES_FILE); errors.Is(err, os.ErrNotExist) {
		// file does not exist
		queryService.Logger.Fatalf("Public queries file <%s> does not exist. Shutting down.", path+models.PUBLIC_QUERIES_FILE)
		return nil
	}

	store, err := implementations.NewPublicQueryStore(queryService.Configuration, queryService.Logger)
	if err != nil {
		queryService.Logger.Fatalf("Failed to initialize PublicQueryStore: %v", err)
		return nil
	}

	PublicQueriesRoutesHelper := &PublicQueriesRoutesHelper{
		ServiceBase: queryService,
		store:       store,
	}

	PublicQueriesRoutesHelper.SetupRoutes(authModel)

	return PublicQueriesRoutesHelper
}

func (s *PublicQueriesRoutesHelper) SetupRoutes(authModel *security.AuthModel) {
	s.Logger.Infof("-----------------------------------------------")
	s.Logger.Infof("Unsecured (public) routes in this query service are:")

	var routeString = "/v1/public/queries/{serviceName}/{methodName}"
	s.RegisterRoute(constants.HTTP_GET, routeString, authModel, s.handlePublicQueries)
}

func (s *PublicQueriesRoutesHelper) handlePublicQueries(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		s.Logger.Infof("Elapsed time for request: %v", elapsed)
	}()

	params := mux.Vars(r)
	queryParams := getQueryParams(r)

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

func writeHttpResponse(w http.ResponseWriter, status int, v []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(v)
}
