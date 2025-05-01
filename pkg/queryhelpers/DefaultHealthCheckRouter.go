package queryhelpers

// It is not required to use this helper implementation, but it is provided as a convenience
// since the code is likely to be identical for each noun service.
//

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sbconstants "github.com/geraldhinson/siftd-base/pkg/constants"
	"github.com/geraldhinson/siftd-base/pkg/security"
	"github.com/geraldhinson/siftd-base/pkg/serviceBase"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/constants"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/implementations"
)

type HealthCheckRouter struct {
	*serviceBase.ServiceBase
	store      *implementations.BaseQueryStore
	debugLevel int
}

func NewHealthCheckRouter(
	service *serviceBase.ServiceBase,
	realm string,
	authType security.AuthTypes,
	timeout security.AuthTimeout,
	approved []string) *HealthCheckRouter {

	var debugLevel = 0
	if service.Configuration.GetString(constants.DEBUGSIFTD_QUERYHELPERS) != "" {
		debugLevel = service.Configuration.GetInt(constants.DEBUGSIFTD_QUERYHELPERS)
	}

	store, err := implementations.NewBaseQueryStore(service.Configuration, service.Logger, "healthcheck:skip-load")
	if err != nil {
		service.Logger.Error("queryservice healthcheck router - failed to initialize query store: ", err)
		return nil
	}

	authModel, err := service.NewAuthModel(realm, authType, timeout, approved)
	if err != nil {
		service.Logger.Error("queryservice healthcheck router - failed to initialize AuthModel with: ", err)
		return nil
	}

	healthCheckRouter := &HealthCheckRouter{
		ServiceBase: service,
		store:       store,
		debugLevel:  debugLevel,
	}

	healthCheckRouter.setupRoutes(authModel)

	return healthCheckRouter
}

func (h *HealthCheckRouter) setupRoutes(authModel *security.AuthModel) {

	var routeString = "/v1/health"
	h.RegisterRoute(constants.HTTP_GET, routeString, authModel, h.GetHealthStandalone)

}

func (h *HealthCheckRouter) GetHealthStandalone(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		if h.debugLevel > 0 {
			h.Logger.Infof("queryservice healthcheck router - Elapsed time for request: %v", elapsed)
		}
	}()

	var health = serviceBase.HealthStatus{
		Status:           sbconstants.HEALTH_STATUS_HEALTHY,
		DependencyStatus: map[string]string{}}

	if h.debugLevel > 0 {
		h.Logger.Infof("queryservice healthcheck router - incoming healthcheck request: %s", r.URL.Path)
	}

	err := h.store.HealthCheck()
	if err != nil {
		h.Logger.Info("queryservice healthcheck router - the call to the query store HealthCheck() in GetHealthStandalone failed with: ", err)
		health.DependencyStatus["database"] = sbconstants.HEALTH_STATUS_UNHEALTHY
		health.Status = sbconstants.HEALTH_STATUS_UNHEALTHY
	} else {
		health.DependencyStatus["database"] = sbconstants.HEALTH_STATUS_HEALTHY
	}

	err = h.GetListOfCalledServices(&health)
	if err != nil {
		h.Logger.Info("queryservice healthcheck router - failed to retrieve called services in GetHealthStandalone: ", err)
		health.CalledServices = []string{err.Error()}
		health.Status = sbconstants.HEALTH_STATUS_UNHEALTHY
	}

	jsonResults, errmsg := json.Marshal(health)
	if errmsg != nil {
		h.Logger.Info("queryservice healthcheck router - failed to convert health structure to json in GetHealthStandalone: ", errmsg)
		h.WriteHttpError(w, sbconstants.RESOURCE_INTERNAL_ERROR_CODE, errmsg)
		return
	}

	h.WriteHttpOK(w, jsonResults)
}

func (h *HealthCheckRouter) GetListOfCalledServices(health *serviceBase.HealthStatus) error {

	calledServices := h.Configuration.GetString(constants.CALLED_SERVICES)
	if calledServices == "" {
		return fmt.Errorf("queryservice healthcheck router - called services not defined in env var: %s", constants.CALLED_SERVICES)
	}

	// Unmarshal the JSON array
	if err := json.Unmarshal([]byte(calledServices), &health.CalledServices); err != nil {
		return fmt.Errorf("queryservice healthcheck router - unmarshalling of called services JSON from env var failed with %w", err)
	}

	return nil
}
