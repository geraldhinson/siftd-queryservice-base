package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"
)

// QueryParam represents a query parameter used in a query.
type QueryParam struct {
	Name     string
	Type     DataType
	Optional bool
}

// Method represents the method that can be called.
type Method struct {
	Enabled         bool
	AuthRequired    []string
	Description     string
	ExampleCall     string
	ServiceName     string
	MethodName      string
	MethodType      MethodType // Assuming MethodType is already defined in your enums (as we discussed earlier)
	Query           string
	QueryParameters []QueryParam // Assuming QueryParam is another struct that represents query parameters
}

// GetQueryParameterNames returns the names of the query parameters, optionally filtering by required parameters.
func (m *Method) GetQueryParameterNames(onlyRequired bool) []string {
	var response []string
	if len(m.QueryParameters) > 0 {
		for _, param := range m.QueryParameters {
			if onlyRequired {
				if !param.Optional {
					response = append(response, param.Name)
				}
			} else {
				response = append(response, param.Name)
			}
		}
	}
	return response
}

// GetMapOfParametersForQueryCall converts the provided call parameters into a map
// suitable for use in a PostgreSQL query. It handles array types by unmarshalling
// JSON strings into Go slices.
func (m *Method) GetMapOfParametersForQueryCall(callParams map[string]string) (pgx.NamedArgs, error) {
	// Create a map of parameters for the query call
	paramMap := pgx.NamedArgs{}
	for _, queryParam := range m.QueryParameters {
		switch queryParam.Type {
		case ARRAY_VARCHAR:
			// Convert the string to an array of strings
			var stringArray []string
			err := json.Unmarshal([]byte(callParams[queryParam.Name]), &stringArray)
			if err != nil {
				return nil, fmt.Errorf("queryservice models - error unmarshalling array of strings for parameter %s: %v", queryParam.Name, err)
			}
			paramMap[queryParam.Name] = stringArray

		case ARRAY_INTEGER:
			// Convert the string to an array of strings
			var integerArray []int
			err := json.Unmarshal([]byte(callParams[queryParam.Name]), &integerArray)
			if err != nil {
				return nil, fmt.Errorf("queryservice models - error unmarshalling array of integers for parameter %s: %v", queryParam.Name, err)
			}
			paramMap[queryParam.Name] = integerArray

		case ARRAY_DATE:
			// Convert the string to an array of strings
			var dateArray []pgtype.Date
			err := json.Unmarshal([]byte(callParams[queryParam.Name]), &dateArray)
			if err != nil {
				return nil, fmt.Errorf("queryservice models - error unmarshalling array of dates for parameter %s: %v", queryParam.Name, err)
			}
			paramMap[queryParam.Name] = dateArray

		default:
			// Add the parameter to the map
			// WARNING: using the default here is based on the knowledge that all allowed types may be
			// safely assigned below. This must be reconsidered if new types are added to the DataType enum.
			paramMap[queryParam.Name] = callParams[queryParam.Name]
		}
	}

	return paramMap, nil
}

// GetQueryStringInCallableFormat returns the query string with query parameter placeholders
// in PostgreSQL format.
// It replaces the placeholders in the query string with '@' notation for PostgreSQL.
func (m *Method) GetQueryStringInCallableFormat() string {
	// Start with the original query
	pgQuery := m.Query

	// Replace placeholders with '@' notation for PostgreSQL
	for _, queryParam := range m.QueryParameters {
		pgQuery = strings.ReplaceAll(pgQuery, "{"+queryParam.Name+"}", "@"+queryParam.Name)
	}
	return pgQuery
}

// GetParameterNamesFromQueryString extracts the parameter names from the query string
// using regex. Freaking regex voodoo.  You swear you'll never use it, then... ;)
func (m *Method) GetParameterNamesFromQueryString() []string {
	paramsInQuery := make(map[string]struct{})
	pattern := `\{([a-zA-Z0-9](?:[a-zA-Z0-9_-]*[a-zA-Z0-9])?)\}` // TESTING this as a better pattern
	//	pattern := `\{([a-zA-Z0-9]*)\}`

	// Compile regex pattern
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(m.Query, -1)

	// Add matched parameter names to the set
	for _, match := range matches {
		if len(match) > 1 {
			paramsInQuery[match[1]] = struct{}{}
		}
	}

	// Convert the set to a slice
	var result []string
	for key := range paramsInQuery {
		result = append(result, key)
	}
	return result
}

// ValidateQueryParamsWithQuery validates that the query parameters match the query string and count.
func (m *Method) ValidateQueryParamsWithQuery(logger *logrus.Logger) bool {

	// Get parameters from query string
	paramsInQueryString := m.GetParameterNamesFromQueryString()

	// Validate parameter names
	if len(m.QueryParameters) > 0 {
		validParams := true
		for _, q := range m.QueryParameters {
			exists := false
			for _, param := range paramsInQueryString {
				if param == q.Name {
					exists = true
					break
				}
			}
			if !exists {
				logger.WithFields(logrus.Fields{
					"service": m.ServiceName,
					"method":  m.MethodName,
					"param":   q.Name,
				}).Error("queryservice models - found query definition with param name inconsistency in the queries file.")
				validParams = false
			}
		}
		if !validParams {
			return false
		}
	}

	// Validate parameter counts
	if len(paramsInQueryString) == 0 && len(m.QueryParameters) == 0 {
		logger.WithFields(logrus.Fields{
			"query": m.Query,
		}).Info("queryservice models - added query with no parameters.")
		return true
	}

	if len(paramsInQueryString) == len(m.QueryParameters) {
		logger.WithFields(logrus.Fields{
			"query": m.Query,
		}).Info("queryservice models - added query with matching parameter count.")
		return true
	}

	logger.WithFields(logrus.Fields{
		"service":  m.ServiceName,
		"method":   m.MethodName,
		"query":    m.Query,
		"expected": len(m.QueryParameters),
		"found":    len(paramsInQueryString),
	}).Error("queryservice models - found query definition with inconsistent param count in the queries file.")
	return false
}
