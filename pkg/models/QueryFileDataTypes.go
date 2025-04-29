package models

import (
	"encoding/json"
	"fmt"
)

// DataTypes represents the available data types that can be used for parameters in the queries file(s) (so far).
// Not to be confused with the data types that a query can return. Those are defined in SimpleReader.go.
type DataType int

const (
	BOOLEAN DataType = iota
	SHORT
	INTEGER
	LONG
	STRING
	FLOAT
	DOUBLE
	GUID
	DATE
	TIMESTAMP
	JSON
	ARRAY_VARCHAR
	ARRAY_INTEGER
	ARRAY_DATE
)

// UnmarshalJSON customizes the JSON decoding for DataType, parsing the string into an enum.
func (dt *DataType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("queryservice models - failed to unmarshal JSON for DataType: %w", err)
	}

	// Map the string to the corresponding enum value
	switch s {
	case "BOOLEAN":
		*dt = BOOLEAN
	case "SHORT":
		*dt = SHORT
	case "INTEGER":
		*dt = INTEGER
	case "LONG":
		*dt = LONG
	case "STRING":
		*dt = STRING
	case "FLOAT":
		*dt = FLOAT
	case "DOUBLE":
		*dt = DOUBLE
	case "GUID":
		*dt = GUID
	case "DATE":
		*dt = DATE
	case "TIMESTAMP":
		*dt = TIMESTAMP
	case "JSON":
		*dt = JSON
	case "ARRAY_VARCHAR":
		*dt = ARRAY_VARCHAR
	case "ARRAY_INTEGER":
		*dt = ARRAY_INTEGER
	case "ARRAY_DATE":
		*dt = ARRAY_DATE
	default:
		return fmt.Errorf("queryservice models - invalid query parameter data type detected in UnmarshalJson: %s", s)
	}
	return nil
}

// MethodType represents the type of request method. Only STANDALONE_REQUEST is supported so far.
type MethodType int

const (
	STANDALONE_REQUEST MethodType = iota
)

// UnmarshalJSON customizes the JSON decoding for MethodType, parsing the string into an enum.
func (mt *MethodType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("queryservice models - failed to unmarshal JSON for MethodType: %w", err)
	}

	// Map the string to the corresponding enum value
	switch s {
	case "STANDALONE_REQUEST":
		*mt = STANDALONE_REQUEST
	default:
		return fmt.Errorf("queryservice models - invalid MethodType %s detected on query", s)
	}
	return nil
}
