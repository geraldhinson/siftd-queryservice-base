package models

const (
	DEBUGTRACE            = false
	DB_CONNECTION_STRING  = "DB_CONNECTSTRING"
	QUERIES_FILE          = "/Resources/Queries.json"
	PUBLIC_QUERIES_FILE   = "/Resources/Public.Queries.json"
	INTERNAL_SERVER_ERROR = "Internal Server Error: "
	NPG_EXCEPTION_MESSAGE = "Postgres Error detected while calling: %s\n\t Error - %s See https://www.postgresql.org/docs/current/errcodes-appendix.html for additional details"
)
