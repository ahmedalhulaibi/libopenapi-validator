package parameters

import (
	"net/http"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
)

func TestLabelStylePathParam(t *testing.T) {
	spec := `openapi: 3.1.0
paths:
  /test/{color}:
    get:
      parameters:
        - name: color
          in: path
          required: true
          style: label
          schema:
            type: string
            minLength: 1
            maxLength: 10
      operationId: testOp`

	doc, _ := libopenapi.NewDocument([]byte(spec))
	m, _ := doc.BuildV3Model()
	v := NewParameterValidator(&m.Model)

	// label style: /test/.blue
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/test/.blue", nil)
	valid, errors := v.ValidatePathParams(req)
	t.Logf("valid: %v, errors: %v", valid, errors)
	assert.True(t, valid, "label style '.blue' should be valid: %v", errors)
}

func TestMatrixStylePathParam(t *testing.T) {
	spec := `openapi: 3.1.0
paths:
  /test/{color}:
    get:
      parameters:
        - name: color
          in: path
          required: true
          style: matrix
          schema:
            type: string
            minLength: 1
            maxLength: 10
      operationId: testOp`

	doc, _ := libopenapi.NewDocument([]byte(spec))
	m, _ := doc.BuildV3Model()
	v := NewParameterValidator(&m.Model)

	// matrix style: /test/;color=blue
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/test/;color=blue", nil)
	valid, errors := v.ValidatePathParams(req)
	t.Logf("valid: %v, errors: %v", valid, errors)
	assert.True(t, valid, "matrix style ';color=blue' should be valid: %v", errors)
}
