module github.com/ethpandaops/pandafuzz/test/api/integration

go 1.21

replace github.com/ethpandaops/pandafuzz => ../../..

require (
	github.com/ethpandaops/pandafuzz v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.4.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.8.4
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)