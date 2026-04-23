// Copyright 2025 Princess Beef Heavy Industries, LLC / Dave Shanley
// SPDX-License-Identifier: MIT

package validator

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMixedEnumTypes_OAS31_SumType tests whether libopenapi-validator handles
// OpenAPI 3.1 sum types: type: [string, boolean] + mixed enum.
func TestMixedEnumTypes_OAS31_SumType(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info: { title: Test, version: 1.0.0 }
paths:
  /t:
    post:
      operationId: op
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [status]
              properties:
                status:
                  type: [string, boolean]
                  enum: ["UNKNOWN", true, false, "PASSING"]
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: [string, boolean]
                    enum: ["UNKNOWN", true, false, "PASSING"]
`)

	doc, err := libopenapi.NewDocument(spec)
	require.NoError(t, err)

	v, errs := NewValidator(doc)
	require.Empty(t, errs)

	tests := []struct {
		name      string
		body      string
		wantValid bool
	}{
		{"string_UNKNOWN", `{"status": "UNKNOWN"}`, true},
		{"string_PASSING", `{"status": "PASSING"}`, true},
		{"boolean_true", `{"status": true}`, true},
		{"boolean_false", `{"status": false}`, true},
		{"string_true", `{"status": "true"}`, false},   // not in enum
		{"string_INVALID", `{"status": "INVALID"}`, false}, // not in enum
		{"integer_1", `{"status": 1}`, false},           // wrong type
	}

	for _, tc := range tests {
		t.Run(tc.name+"_request", func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/t", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")

			valid, validErrs := v.ValidateHttpRequest(req)
			if tc.wantValid {
				assert.True(t, valid, "should be valid")
			} else {
				assert.False(t, valid, "should be invalid")
			}
			if !valid {
				for _, ve := range validErrs {
					for _, se := range ve.SchemaValidationErrors {
						t.Logf("  error: %s", se.Reason)
					}
				}
			}
		})

		t.Run(tc.name+"_response", func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/t", nil)
			resp := &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(tc.body))),
			}

			valid, validErrs := v.ValidateHttpResponse(req, resp)
			if tc.wantValid {
				assert.True(t, valid, "should be valid")
			} else {
				assert.False(t, valid, "should be invalid")
			}
			if !valid {
				for _, ve := range validErrs {
					for _, se := range ve.SchemaValidationErrors {
						t.Logf("  error: %s", se.Reason)
					}
				}
			}
		})
	}
}

// TestMixedEnumTypes probes how libopenapi-validator handles mixed-type enums:
// type: string + enum: ["UNKNOWN", true, false, "PASSING"].
// JSON Schema says both type AND enum must be satisfied. Only string enum
// values should pass; boolean enum values should fail the type check.
func TestMixedEnumTypes(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info: { title: Test, version: 1.0.0 }
paths:
  /t:
    post:
      operationId: op
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [status]
              properties:
                status:
                  type: string
                  enum: ["UNKNOWN", true, false, "PASSING"]
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
                    enum: ["UNKNOWN", true, false, "PASSING"]
`)

	doc, err := libopenapi.NewDocument(spec)
	require.NoError(t, err)

	v, errs := NewValidator(doc)
	require.Empty(t, errs)

	tests := []struct {
		name     string
		body     string
		wantValid bool
		desc     string
	}{
		{
			name:      "string_enum_value",
			body:      `{"status": "UNKNOWN"}`,
			wantValid: true,
			desc:      "string value in enum should pass",
		},
		{
			name:      "string_PASSING",
			body:      `{"status": "PASSING"}`,
			wantValid: true,
			desc:      "another string value in enum should pass",
		},
		{
			name:      "boolean_true",
			body:      `{"status": true}`,
			wantValid: false,
			desc:      "boolean true: in enum but wrong type (type: string)",
		},
		{
			name:      "boolean_false",
			body:      `{"status": false}`,
			wantValid: false,
			desc:      "boolean false: in enum but wrong type (type: string)",
		},
		{
			name:      "string_true",
			body:      `{"status": "true"}`,
			wantValid: true,
			desc:      "string 'true': OAS 3.0 enum coercion converts boolean true to string",
		},
		{
			name:      "string_false",
			body:      `{"status": "false"}`,
			wantValid: true,
			desc:      "string 'false': OAS 3.0 enum coercion converts boolean false to string",
		},
		{
			name:      "string_not_in_enum",
			body:      `{"status": "INVALID"}`,
			wantValid: false,
			desc:      "string not in enum should fail",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_request", func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/t", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")

			valid, validErrs := v.ValidateHttpRequest(req)
			if tc.wantValid {
				assert.True(t, valid, "request should be valid: %s", tc.desc)
			} else {
				assert.False(t, valid, "request should be invalid: %s", tc.desc)
			}
			if !valid {
				for _, ve := range validErrs {
					for _, se := range ve.SchemaValidationErrors {
						t.Logf("  error: %s", se.Reason)
					}
				}
			}
		})

		t.Run(tc.name+"_response", func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/t", nil)
			resp := &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(tc.body))),
			}

			valid, validErrs := v.ValidateHttpResponse(req, resp)
			if tc.wantValid {
				assert.True(t, valid, "response should be valid: %s", tc.desc)
			} else {
				assert.False(t, valid, "response should be invalid: %s", tc.desc)
			}
			if !valid {
				for _, ve := range validErrs {
					for _, se := range ve.SchemaValidationErrors {
						t.Logf("  error: %s", se.Reason)
					}
				}
			}
		})
	}
}

// TestAdditionalPropertiesWithAllOfInheritance tests whether the validator
// rejects derived-type properties when the base type has
// additionalProperties: {type: object}. This is the windows.net/graphrbac
// pattern: DirectoryObject has additionalProperties: {type: object}, and
// Application extends it via allOf with string properties.
func TestAdditionalPropertiesWithAllOfInheritance(t *testing.T) {
	spec := []byte(`
openapi: 3.0.3
info: { title: Test, version: 1.0.0 }
paths:
  /t:
    get:
      operationId: op
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Application'
components:
  schemas:
    DirectoryObject:
      type: object
      additionalProperties:
        type: object
      properties:
        objectId:
          type: string
        objectType:
          type: string
    Application:
      allOf:
        - $ref: '#/components/schemas/DirectoryObject'
      properties:
        displayName:
          type: string
        appId:
          type: string
        accountEnabled:
          type: boolean
`)

	doc, err := libopenapi.NewDocument(spec)
	require.NoError(t, err)

	v, errs := NewValidator(doc)
	require.Empty(t, errs)

	// A valid Application object with base + derived properties
	body := `{"objectId": "123", "objectType": "Application", "displayName": "MyApp", "appId": "abc", "accountEnabled": true}`

	req, _ := http.NewRequest("GET", "/t", nil)
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}

	valid, validErrs := v.ValidateHttpResponse(req, resp)
	for _, ve := range validErrs {
		for _, se := range ve.SchemaValidationErrors {
			t.Logf("error: %s", se.Reason)
		}
	}

	// Document current behavior — does the validator reject this?
	if valid {
		t.Log("PASS: validator accepts derived properties alongside additionalProperties: {type: object}")
	} else {
		t.Log("FAIL: validator rejects derived properties — additionalProperties on base conflicts with allOf extension")
	}
}
