package implementations

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/sirupsen/logrus"
)

type SimpleReader struct {
	logger     *logrus.Logger
	rows       pgx.Rows
	debugLevel int
}

// NewSimpleReader initializes a new SimpleReader
func NewSimpleReader(rows pgx.Rows, logger *logrus.Logger, debugLevel int) *SimpleReader {
	return &SimpleReader{
		rows:       rows,
		logger:     logger,
		debugLevel: debugLevel,
	}
}

// Release closes the connection and rows when done
func (sr *SimpleReader) Release() {
	sr.rows.Close()
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

	values, err := sr.rows.Values()
	if err != nil {
		return err
	}

	// Retrieve the type and value of the column
	fieldType := sr.rows.FieldDescriptions()[column].DataTypeOID

	if sr.debugLevel > 1 {
		sr.logger.Infof("name: %v\n", sr.GetFieldName(column))
		sr.logger.Infof("type: %v\n", fieldType)
		sr.logger.Infof("val: %v\n", values[column])
	}

	// TODO: Add support/un-support for more data types
	// TODO: are nulls (from nullable columns) being handled correctly? if not, add something like this:
	//  if values[column] == nil {
	//	  columnDictionary[sr.GetFieldName(column)] = nil
	//	  return nil
	//  }
	switch fieldType {

	// explicitly not supported list (so far)
	case pgtype.UnknownOID, pgtype.XMLOID:
		return fmt.Errorf("queryservice store - Unsupported field type found in fetched row: %v", fieldType)

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
		uuidArray, ok := values[column].([16]uint8)
		if !ok {
			return fmt.Errorf("queryservice store - invalid UUID value %v detected", values[column])
		}

		uuidValue, err := uuid.FromBytes(uuidArray[:])
		if err != nil {
			return fmt.Errorf("queryservice store - unable to convert stored uuid value to a string equivalent: %v\n", err)
		}
		columnDictionary[sr.GetFieldName(column)] = uuidValue.String()

	// optimistic default case for things not tested so far. This is questionable, but so far
	// the default behavior has worked very well, so leaving it for now.
	default:
		sr.logger.Infof("queryservice store - default assignment of field type used in GetFieldValue(). Consider adding explicit case for this type: %v", fieldType)
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

// PrintAllResults prints all rows for debugging purposes (unused currently)
func (sr *SimpleReader) PrintAllResults(logger *log.Logger) error {

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
