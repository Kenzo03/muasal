// Package httpapi serves the /api/v1 REST API defined in api/openapi.yaml.
// api.gen.go is generated from that file; run `go generate ./...` after editing it.
package httpapi

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config oapi-codegen.yaml ../../../api/openapi.yaml
