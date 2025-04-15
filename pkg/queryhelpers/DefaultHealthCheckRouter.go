package queryhelpers

// It is not required to use this helper implementation, but it is provided as a convenience
// since the code is likely to be identical for each noun service.
//

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/geraldhinson/siftd-base/pkg/constants"
	"github.com/geraldhinson/siftd-base/pkg/security"
	"github.com/geraldhinson/siftd-base/pkg/serviceBase"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/implementations"
)

type HealthCheckRouter struct {
	*serviceBase.ServiceBase
	store *implementations.BaseQueryStore
}

func NewHealthCheckRouter(
	service *serviceBase.ServiceBase,
	realm string,
	authType security.AuthTypes,
	timeout security.AuthTimeout,
	approved []string) *HealthCheckRouter {

	store, err := implementations.NewPublicQueryStore(service.Configuration, service.Logger)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize default HealthCheckRouter for query service: %v", err)
		return nil
	}

	authModel, err := service.NewAuthModel(realm, authType, timeout, approved)
	if err != nil {
		service.Logger.Fatalf("Failed to initialize AuthModelUsers in default HealthCheckRouter for query service: %v", err)
		return nil
	}

	healthCheckRouter := &HealthCheckRouter{
		ServiceBase: service,
		store:       store,
	}
	healthCheckRouter.setupRoutes(authModel)

	return healthCheckRouter
}

func (h *HealthCheckRouter) setupRoutes(authModel *security.AuthModel) {

	var routeString = "/v1/health"
	h.RegisterRoute(constants.HTTP_GET, routeString, authModel, h.GetHealthStandalone)

}

func (h *HealthCheckRouter) GetHealthStandalone(w http.ResponseWriter, r *http.Request) {
	var health = serviceBase.HealthStatus{
		Status:           constants.HEALTH_STATUS_HEALTHY,
		DependencyStatus: map[string]string{}}

	//		err := h.store.HealthCheck() // TODO: decided what to call here for query service health of db
	var err = fmt.Errorf("unimplemented db health check! Fix.") // TODO: decided what to call here for query service health of db
	if err != nil {
		h.Logger.Info("Call to HealthCheck in GetHealthStandalone failed with: ", err)
		health.DependencyStatus["database"] = constants.HEALTH_STATUS_UNHEALTHY
		health.Status = constants.HEALTH_STATUS_UNHEALTHY
	} else {
		health.DependencyStatus["database"] = constants.HEALTH_STATUS_HEALTHY
	}

	// TODO: fix this experiment along with the method below
	h.GetListOfCalledServices(&health)

	jsonResults, errmsg := json.Marshal(health)
	if errmsg != nil {
		h.Logger.Info("Failed to convert health structure to json: ", errmsg)
		h.WriteHttpError(w, constants.RESOURCE_INTERNAL_ERROR_CODE, errmsg)
		return
	}

	h.WriteHttpOK(w, jsonResults)
}

func (h *HealthCheckRouter) GetListOfCalledServices(health *serviceBase.HealthStatus) {
	// TODO: implement this method
	calledServices := h.Configuration.GetString(constants.CALLED_SERVICES)

	// Declare a slice to hold the parsed array
	//	var stringArray []string

	// Unmarshal the JSON array
	if err := json.Unmarshal([]byte(calledServices), &health.CalledServices); err != nil {
		fmt.Println("failed in GetListOfCalledServices unmarshalling called services JSON from env var:", err)
		return
	}
}
