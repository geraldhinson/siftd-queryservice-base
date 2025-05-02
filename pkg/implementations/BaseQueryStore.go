package implementations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/geraldhinson/siftd-queryservice-base/pkg/constants"
	"github.com/geraldhinson/siftd-queryservice-base/pkg/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// type BaseQueryStore[T interfaces.IQueryStore] struct {
type BaseQueryStore struct {
	dbConnectString string
	Methods         []models.Method
	logger          *logrus.Logger
	dbPool          *pgxpool.Pool
	rootCtx         *context.Context
	cancel          *context.CancelFunc
	debugLevel      int
}

// NewPrivateQueryStore is the constructor for PrivateQueryStore, similar to the C# constructor
func NewPrivateQueryStore(configuration *viper.Viper, logger *logrus.Logger) (*BaseQueryStore, error) {

	path := configuration.GetString("RESDIR_PATH")
	logger.Info("queryservice store - Path: ", path)

	// Create a new PrivateQueryStore by passing necessary arguments to the base class constructor
	store, err := NewBaseQueryStore(configuration, logger, path+constants.QUERIES_FILE)
	if err != nil {
		return nil, err
	}

	// Return the initialized PrivateQueryStore instance
	return store, nil
}

// NewPublicQueryStore is the constructor for PublicQueryStore, similar to the C# constructor
func NewPublicQueryStore(configuration *viper.Viper, logger *logrus.Logger) (*BaseQueryStore, error) {

	path := configuration.GetString("RESDIR_PATH")
	logger.Info("queryservice store - Path: ", path)

	// Create a new PublicQueryStore by passing necessary arguments to the base class constructor
	store, err := NewBaseQueryStore(configuration, logger, path+constants.PUBLIC_QUERIES_FILE)
	if err != nil {
		return nil, err
	}

	// Return the initialized PublicQueryStore instance
	return store, nil
}

// private methods below here
func NewBaseQueryStore(configuration *viper.Viper, logger *logrus.Logger, fileName string) (*BaseQueryStore, error) {

	var debugLevel = 0
	if configuration.GetString(constants.DEBUGSIFTD_QUERYSTORE) != "" {
		debugLevel = configuration.GetInt(constants.DEBUGSIFTD_QUERYSTORE)
	}

	store := &BaseQueryStore{logger: logger, debugLevel: debugLevel}

	store.dbConnectString = configuration.GetString(constants.DB_CONNECTION_STRING)
	if store.dbConnectString == "" {
		return nil, fmt.Errorf("queryservice store - unable to retrieve database connection string")
	}

	// Initialize the database pool (example with pgx)
	connConfig, err := pgxpool.ParseConfig(store.dbConnectString)
	if err != nil {
		return nil, fmt.Errorf("queryservice store - unable to parse connection config: %v", err)
	}
	rootCtx, cancel := context.WithCancel(context.Background())
	store.rootCtx = &rootCtx
	store.cancel = &cancel

	connConfig.MaxConnIdleTime = 60 * time.Second
	connConfig.MaxConnLifetime = 60 * time.Second
	connConfig.MaxConns = 15

	//	defer cancel()

	store.dbPool, err = pgxpool.NewWithConfig(*store.rootCtx, connConfig)
	if err != nil {
		return nil, fmt.Errorf("queryservice store - unable to connect to database: %v", err)
	}
	//	defer store.dbPool.Close()

	// Verify the connection
	err = store.dbPool.Ping(*store.rootCtx)
	if err != nil {
		return nil, fmt.Errorf("queryservice store - unable to ping database: %w", err)
	}
	logger.Info("queryservice store - successfully connected to database")

	if !(fileName == "healthcheck:skip-load") {
		if store.debugLevel > 0 {
			logger.Info("queryservice store - Loading queries from file: ", fileName)
		}

		err = store.loadQueries(fileName)
		if err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (store *BaseQueryStore) loadQueries(queryFile string) error {
	fileData, err := os.ReadFile(queryFile)
	if err != nil {
		store.logger.Infof("queryservice store - error loading query file: %v", err)
		return err
	}

	var methods []models.Method
	err = json.Unmarshal(fileData, &methods)
	if err != nil {
		store.logger.Infof("queryservice store - error unmarshalling query file: %v", err)
		return err
	}

	for _, m := range methods {
		if m.ValidateQueryParamsWithQuery(store.logger) {
			store.Methods = append(store.Methods, m)
		} else {
			store.logger.Infof("queryservice store - query params validation failed for method: %s", m.MethodName)
		}
	}

	return nil
}

func (store *BaseQueryStore) GetQueryList() ([]byte, error) {
	// Return the list of queries as json
	jsonData, err := json.Marshal(store.Methods)
	if err != nil {
		store.logger.Info("queryservice store - error marshalling query list: ", err)
		return nil, fmt.Errorf("queryservice store - error marshalling query list: %w", err)
	}

	return jsonData, nil
}

func (store *BaseQueryStore) RunStandAloneQuery(
	serviceName string,
	methodName string,
	callParameters map[string]string) ([]byte, error) {

	serviceName = strings.TrimSpace(serviceName)
	methodName = strings.TrimSpace(methodName)

	if store.debugLevel > 1 {
		store.monitorPoolStats()
	}

	// Lookup the query to see if it exists
	var method *models.Method
	for _, m := range store.Methods {
		if m.ServiceName == serviceName && m.MethodName == methodName {
			method = &m
			break
		}
	}

	if method == nil {
		return nil, fmt.Errorf("queryservice store - unable to run the undefined service/method requested: %s/%s", serviceName, methodName)
	}

	// Validate required parameters
	missingParams := []string{}
	for _, paramName := range method.GetQueryParameterNames(true) {
		if _, exists := callParameters[paramName]; !exists {
			missingParams = append(missingParams, paramName)
		}
	}

	if len(missingParams) > 0 {
		return nil, fmt.Errorf("queryservice store - unable to run request due to missing required parameter(s): %s", strings.Join(missingParams, ", "))
	}

	// Validate extra parameters
	extraParams := []string{}
	for paramName := range callParameters {
		// look for the parameter in the method's query parameters
		foundName := false
		for queryParam := range method.QueryParameters {
			if method.QueryParameters[queryParam].Name == paramName {
				foundName = true
				break
			}
		}
		if !foundName {
			extraParams = append(extraParams, paramName)
		}
	}

	if len(extraParams) > 0 {
		return nil, fmt.Errorf("queryservice store - unable to run request due to invalid input parameter(s) detected on request: %s", strings.Join(extraParams, ", "))
	}

	// Create the SQL query and execute it
	// Multiple rows query
	query := method.GetQueryStringInCallableFormat()
	paramMap, err := method.GetMapOfParametersForQueryCall(callParameters) // TODO_PORT: the called func here needs to return both paramMap and error, then test it
	if err != nil {
		store.logger.Info("queryservice store - error creating parameter map for query: ", err)
		return nil, fmt.Errorf("queryservice store - error creating parameter map for query: %w", err)
	}

	if store.debugLevel > 0 {
		store.logger.Info("queryservice store - Query: ", query)
		store.logger.Info("queryservice store - Query params: ", paramMap)
	}

	rows, err := store.dbPool.Query(*store.rootCtx, query, paramMap)
	// rows, err := store.dbPool.Query(*store.rootCtx, query, ids)
	if err != nil {
		store.logger.Error("queryservice store - error detected on Query call: ", err)
		// We don't pass the database error back to the caller. We log it and return a generic error message.
		// This is to prevent leaking sensitive information to the caller.
		return nil, fmt.Errorf(constants.INTERNAL_SERVER_ERROR + "A backend system error occurred in the queries service. Please check the logs")
	}
	defer rows.Close()

	// Process the query results
	sr := NewSimpleReader(rows, store.logger, store.debugLevel)
	result, err := sr.ProcessResponse()
	if err != nil {
		// build empty result to return to caller
		return []byte("[]"), fmt.Errorf("queryservice store - error encountered while processing query results: %w", err)
	}
	if store.debugLevel > 0 {
		store.logger.Info("queryservice store - Query result: ", result)
	}

	jsonResults, err := json.Marshal(result)
	if err != nil {
		store.logger.Info("queryservice store - failed to marshal valid results returned from query: ", err)
		jsonResults = ([]byte(err.Error()))
		return jsonResults, fmt.Errorf("queryservice store - error encountered marshalling results returned for the query: %w", err)
	}

	// if no error, but no results, we return an empty array with a 200 status
	if string(jsonResults) == "null" {
		if store.debugLevel > 0 {
			store.logger.Info("queryservice store - no results marshalled - forcing empty array")
		}
		jsonResults = []byte("[]")
	}

	//	return result, nil // Replace with actual response from query execution
	return jsonResults, nil // Replace with actual response from query execution
}

func (store *BaseQueryStore) HealthCheck() error {
	store.monitorPoolStats()

	// Verify the connection
	err := store.dbPool.Ping(*store.rootCtx)
	if err != nil {
		return fmt.Errorf("queryservice store - unable to ping database in GetHealth(): %w", err)
	}

	if store.debugLevel > 0 {
		store.logger.Info("queryservice store - HealthCheck successfully connected to database")
	}

	return nil
}

func (store *BaseQueryStore) monitorPoolStats() {
	stats := store.dbPool.Stat()
	statsMap := make(map[string]int)

	statsMap["total_connections"] = int(stats.TotalConns())
	statsMap["acquired_connections"] = int(stats.AcquiredConns())
	statsMap["idle_connections"] = int(stats.IdleConns())
	statsMap["max_connections"] = int(stats.MaxConns())
	statsMap["max_connection_lifetime"] = int(store.dbPool.Config().MaxConnLifetime.Seconds())
	statsMap["max_connection_idle_time"] = int(store.dbPool.Config().MaxConnIdleTime.Seconds())

	store.logger.Info("queryservice store - Pool stats", statsMap)
}

/* TODO: Implement this if needed
func (store *BaseQueryStore[T]) Close() {
	(*store.cancel)()
	store.dbPool.Close()
}
*/
