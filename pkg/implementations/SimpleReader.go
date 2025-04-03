package implementations

import (
	"fmt"
	"log"

	"github.com/geraldhinson/siftd-queryservice-base/pkg/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
)

type SimpleReader struct {
	logger *logrus.Logger
	conn   *pgxpool.Pool
	rows   pgx.Rows
}

// NewSimpleReader initializes a new SimpleReader
func NewSimpleReader(rows pgx.Rows, conn *pgxpool.Pool, logger *logrus.Logger) *SimpleReader {
	return &SimpleReader{
		rows:   rows,
		conn:   conn,
		logger: logger,
	}
}

// Release closes the connection and rows when done
func (sr *SimpleReader) Release() {
	sr.rows.Close()
	sr.conn.Close()
}

// GetFieldCount returns the number of columns in the result set
func (sr *SimpleReader) GetFieldCount() int {
	return len(sr.rows.FieldDescriptions())
}

// GetFieldName retrieves the name of the column at the given index
func (sr *SimpleReader) GetFieldName(column int) string {
	return sr.rows.FieldDescriptions()[column].Name
}

// GetFieldValue reads the value of a specific column and adds it to the column dictionary
func (sr *SimpleReader) GetFieldValue(columnDictionary map[string]interface{}, column int) error {
	// TODO_PORT: Check if the column is NULL
	//	if sr.rows.IsDBNull(column) {
	//		columnDictionary[sr.GetFieldName(column)] = nil
	//		return nil
	//	}

	values, err := sr.rows.Values()
	if err != nil {
		return err
	}

	// Retrieve the type and value of the column
	fieldType := sr.rows.FieldDescriptions()[column].DataTypeOID

	if models.DEBUGTRACE == true {
		sr.logger.Infof("val: %v\n", values[column])
		sr.logger.Infof("type: %v\n", fieldType)
		sr.logger.Infof("name: %v\n", sr.GetFieldName(column))
	}

	// TODO_PORT: Add support/un-support for more data types
	//
	switch fieldType {

	// explicitly not supported list (so far)
	case pgtype.UnknownOID, pgtype.XMLOID:
		return fmt.Errorf("Unsupported field type found in fetched row: %v", fieldType)

	// known to work from testing
	case pgtype.BoolOID,
		pgtype.TextOID, pgtype.VarcharOID, pgtype.VarcharArrayOID,
		pgtype.Int4OID, pgtype.Int8OID, pgtype.Int4ArrayOID, pgtype.Int8ArrayOID,
		pgtype.Float4OID, pgtype.Float8OID,
		pgtype.TimeOID, pgtype.TimestampOID, pgtype.TimestamptzOID,
		pgtype.DateOID, pgtype.DateArrayOID,
		pgtype.JSONOID, pgtype.JSONBOID:

		columnDictionary[sr.GetFieldName(column)] = values[column]

	// things that require special handling
	case pgtype.UUIDOID:
		uuidArray := values[column].([16]uint8)
		uuidValue, err := uuid.FromBytes(uuidArray[:])
		if err != nil {
			return fmt.Errorf("Unable to convert stored uuid value to a string equivalent: %v\n", err)
		}
		columnDictionary[sr.GetFieldName(column)] = uuidValue.String()

	// optimistic default case for things not tested so far
	default:
		columnDictionary[sr.GetFieldName(column)] = values[column]
	}

	// Add the value to the dictionary
	//	columnDictionary[sr.GetFieldName(column)] = value
	return nil
}

// ProcessResponse reads all rows and processes each row into a list of dictionaries
func (sr *SimpleReader) ProcessResponse() ([]map[string]interface{}, error) {
	var result []map[string]interface{}

	for sr.rows.Next() {
		columnDictionary := make(map[string]interface{})
		for column := 0; column < sr.GetFieldCount(); column++ {
			if err := sr.GetFieldValue(columnDictionary, column); err != nil {
				return nil, err
			}
		}
		result = append(result, columnDictionary)
	}

	if err := sr.rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// PrintAllResults prints all rows for debugging purposes
func (sr *SimpleReader) PrintAllResults(logger *log.Logger) error {

	// TODO_PORT: fix this recursive call (it was showing sr.rows.Next as recursive)?
	for sr.rows.Next() {
		values, err := sr.rows.Values()
		if err != nil {
			return err
		}

		for column := 0; column < sr.GetFieldCount(); column++ {
			fieldName := sr.GetFieldName(column)
			fieldValue := values[column]
			sr.logger.Infof("%s: %v", fieldName, fieldValue)
		}
	}
	return sr.rows.Err()
}

// TODO_PORT: add support somewhere for more functionality of pgx pools
// - stats to see if the pool is being used correctly (e.g. if connections are being released)
// - error handling for when the pool is full
// - maybe explicit allowed connections for the pool
// - etc. (hit enter after these for more suggestion from copilot)
// - add support for more data types in GetFieldValue
