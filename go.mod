module github.com/kdev1966/cobol-api

go 1.25.0

// Les versions anterieures a 1.26.6 portent quatre vulnerabilites de la
// bibliotheque standard que ce service atteint, dans net/http et crypto/tls.
toolchain go1.26.6

require (
	github.com/jackc/pgx/v5 v5.11.0
	golang.org/x/time v0.14.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
