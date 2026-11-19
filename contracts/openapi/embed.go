// Package openapi embebe el contrato de la API pública para que el Gateway lo sirva.
package openapi

import _ "embed"

// Spec es el contenido de openapi.yaml.
//
//go:embed openapi.yaml
var Spec []byte
