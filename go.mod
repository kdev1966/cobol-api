module github.com/kdev1966/cobol-api

go 1.24.0

// Les versions anterieures a 1.26.6 portent quatre vulnerabilites de la
// bibliotheque standard que ce service atteint, dans net/http et crypto/tls.
toolchain go1.26.6

require golang.org/x/time v0.14.0
