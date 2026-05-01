package schema_validation

import (
	"encoding/json"
	"testing"

	"github.com/pb33f/libopenapi"
)

// TestTransformXMLToSchemaJSON_NestedAttrs traces the XML-to-JSON
// transformation for nested self-closing elements with attributes
// to identify where properties get lost.
func TestTransformXMLToSchemaJSON_NestedAttrs(t *testing.T) {
	specBytes := []byte(`{
  "openapi": "3.1.0",
  "info": {"title": "test", "version": "1"},
  "paths": {},
  "components": {
    "schemas": {
      "Certificate": {
        "type": "object",
        "xml": { "name": "Certificate" },
        "required": ["language", "name", "IssuedBy", "CertificateData"],
        "properties": {
          "language": {
            "type": "string",
            "xml": { "attribute": true }
          },
          "name": {
            "type": "string",
            "xml": { "attribute": true }
          },
          "IssuedBy": {
            "type": "object",
            "required": ["Organization"],
            "properties": {
              "Organization": {
                "type": "object",
                "required": ["name", "code"],
                "properties": {
                  "name": {
                    "type": "string",
                    "xml": { "attribute": true }
                  },
                  "code": {
                    "type": "string",
                    "xml": { "attribute": true }
                  }
                }
              }
            }
          },
          "CertificateData": {
            "type": "object",
            "required": ["Examination"],
            "properties": {
              "Examination": {
                "type": "object",
                "required": ["subject", "year"],
                "properties": {
                  "subject": {
                    "type": "string",
                    "xml": { "attribute": true }
                  },
                  "year": {
                    "type": "string",
                    "xml": { "attribute": true }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`)

	doc, err := libopenapi.NewDocument(specBytes)
	if err != nil {
		t.Fatal(err)
	}

	model, err2 := doc.BuildV3Model()
	if err2 != nil {
		t.Fatal(err2)
	}

	schemaProxy := model.Model.Components.Schemas.GetOrZero("Certificate")
	schema := schemaProxy.Schema()

	xml := `<Certificate language="en" name="Test"><IssuedBy><Organization name="CBSE" code="cbse"/></IssuedBy><CertificateData><Examination subject="Math" year="2024"/></CertificateData></Certificate>`

	t.Logf("input xml: %s", xml)

	result, valErrs := TransformXMLToSchemaJSON(xml, schema)
	if len(valErrs) > 0 {
		for _, ve := range valErrs {
			t.Logf("pre-validation error: %s", ve.Message)
		}
	}

	pretty, _ := json.MarshalIndent(result, "", "  ")
	t.Logf("transformed json:\n%s", pretty)

	// Assert the structure is correct.
	rootMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}

	// Root attributes should be converted (- prefix stripped).
	if _, ok := rootMap["language"]; !ok {
		t.Error("root attribute 'language' missing")
	}

	if _, ok := rootMap["name"]; !ok {
		t.Error("root attribute 'name' missing")
	}

	// IssuedBy should exist and contain Organization.
	issuedBy, ok := rootMap["IssuedBy"].(map[string]any)
	if !ok {
		t.Fatalf("IssuedBy missing or not a map: %v", rootMap["IssuedBy"])
	}

	org, ok := issuedBy["Organization"].(map[string]any)
	if !ok {
		t.Errorf("IssuedBy.Organization missing or not a map — got keys: %v", mapKeys(issuedBy))
		t.Errorf("BUG: single-child unwrap in applyXMLTransformations strips the Organization wrapper")
	} else {
		if _, ok := org["name"]; !ok {
			t.Error("Organization.name missing")
		}
		if _, ok := org["code"]; !ok {
			t.Error("Organization.code missing")
		}
	}

	// CertificateData should exist and contain Examination.
	certData, ok := rootMap["CertificateData"].(map[string]any)
	if !ok {
		t.Fatalf("CertificateData missing or not a map: %v", rootMap["CertificateData"])
	}

	exam, ok := certData["Examination"].(map[string]any)
	if !ok {
		t.Errorf("CertificateData.Examination missing or not a map — got keys: %v", mapKeys(certData))
		t.Errorf("BUG: single-child unwrap in applyXMLTransformations strips the Examination wrapper")
	} else {
		if _, ok := exam["subject"]; !ok {
			t.Error("Examination.subject missing")
		}
		if _, ok := exam["year"]; !ok {
			t.Error("Examination.year missing")
		}
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestTransformXMLToSchemaJSON_DrivingLicense reproduces the "got string, want
// object" failure from the apisetu corpus. Uses the DrivingLicenseSchema structure
// with deeply nested attribute-only objects (Address, Photo).
func TestTransformXMLToSchemaJSON_DrivingLicense(t *testing.T) {
	specBytes := []byte(`{
  "openapi": "3.0.0",
  "info": {"title": "test", "version": "1"},
  "paths": {},
  "components": {
    "schemas": {
      "DrivingLicenseSchema": {
        "type": "object",
        "xml": { "name": "Certificate" },
        "required": ["language", "name", "type", "number", "IssuedBy", "IssuedTo", "CertificateData"],
        "properties": {
          "language":      { "type": "string", "xml": { "attribute": true } },
          "name":          { "type": "string", "xml": { "attribute": true } },
          "number":        { "type": "integer", "xml": { "attribute": true } },
          "type":          { "type": "string", "xml": { "attribute": true } },
          "status":        { "type": "string", "xml": { "attribute": true } },
          "CertificateData": {
            "type": "object",
            "properties": {
              "DrivingLicense": {
                "type": "object",
                "required": ["code"],
                "properties": {
                  "code":        { "type": "string", "xml": { "attribute": true } },
                  "description": { "type": "string", "xml": { "attribute": true } }
                }
              }
            }
          },
          "IssuedBy": {
            "type": "object",
            "required": ["Organization"],
            "properties": {
              "Organization": {
                "type": "object",
                "required": ["name", "code", "Address"],
                "properties": {
                  "code": { "type": "string", "xml": { "attribute": true } },
                  "name": { "type": "string", "xml": { "attribute": true } },
                  "type": { "type": "string", "xml": { "attribute": true } },
                  "Address": {
                    "type": "object",
                    "required": ["country"],
                    "properties": {
                      "country": { "type": "string", "xml": { "attribute": true } },
                      "state":   { "type": "string", "xml": { "attribute": true } },
                      "type":    { "type": "string", "xml": { "attribute": true } }
                    }
                  }
                }
              }
            }
          },
          "IssuedTo": {
            "type": "object",
            "required": ["Person"],
            "properties": {
              "Person": {
                "type": "object",
                "required": ["name", "dob", "Address", "Photo"],
                "properties": {
                  "name": { "type": "string", "xml": { "attribute": true } },
                  "dob":  { "type": "string", "xml": { "attribute": true } },
                  "Address": {
                    "type": "object",
                    "required": ["country"],
                    "properties": {
                      "country": { "type": "string", "xml": { "attribute": true } },
                      "state":   { "type": "string", "xml": { "attribute": true } },
                      "type":    { "type": "string", "xml": { "attribute": true } }
                    }
                  },
                  "Photo": {
                    "type": "object",
                    "required": ["format"],
                    "properties": {
                      "format": { "type": "string", "xml": { "attribute": true } }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`)

	doc, err := libopenapi.NewDocument(specBytes)
	if err != nil {
		t.Fatal(err)
	}

	model, err2 := doc.BuildV3Model()
	if err2 != nil {
		t.Fatal(err2)
	}

	schemaProxy := model.Model.Components.Schemas.GetOrZero("DrivingLicenseSchema")
	schema := schemaProxy.Schema()

	xml := `<Certificate language="en" name="DL" number="17" type="DRVLC" status="A">
  <CertificateData>
    <DrivingLicense code="NT" description="Motor Cycle"/>
  </CertificateData>
  <IssuedBy>
    <Organization code="p" name="RTO" type="CG">
      <Address country="IN" state="" type="present"/>
    </Organization>
  </IssuedBy>
  <IssuedTo>
    <Person name="ANAND" dob="1988-10-01">
      <Address country="IN" state="" type="permanent"/>
      <Photo format="jpg"/>
    </Person>
  </IssuedTo>
</Certificate>`

	t.Logf("input xml:\n%s", xml)

	result, valErrs := TransformXMLToSchemaJSON(xml, schema)
	for _, ve := range valErrs {
		t.Logf("transform error: %s", ve.Message)
	}

	pretty, _ := json.MarshalIndent(result, "", "  ")
	t.Logf("transformed json:\n%s", pretty)

	// Walk the transformed JSON and check that objects are maps, not strings.
	rootMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("root: expected map, got %T: %v", result, result)
	}

	// IssuedBy.Organization.Address should be a map.
	issuedBy, _ := rootMap["IssuedBy"].(map[string]any)
	if issuedBy == nil {
		t.Fatal("IssuedBy missing or not a map")
	}
	org, _ := issuedBy["Organization"].(map[string]any)
	if org == nil {
		t.Fatalf("Organization missing or not a map, IssuedBy keys: %v", mapKeys(issuedBy))
	}
	addr, ok := org["Address"].(map[string]any)
	if !ok {
		t.Errorf("Organization.Address: got %T (%v), want map", org["Address"], org["Address"])
	} else {
		t.Logf("Organization.Address keys: %v", mapKeys(addr))
	}

	// IssuedTo.Person.Address should be a map.
	issuedTo, _ := rootMap["IssuedTo"].(map[string]any)
	if issuedTo == nil {
		t.Fatal("IssuedTo missing or not a map")
	}
	person, _ := issuedTo["Person"].(map[string]any)
	if person == nil {
		t.Fatalf("Person missing or not a map, IssuedTo keys: %v", mapKeys(issuedTo))
	}
	pAddr, ok := person["Address"].(map[string]any)
	if !ok {
		t.Errorf("Person.Address: got %T (%v), want map", person["Address"], person["Address"])
	} else {
		t.Logf("Person.Address keys: %v", mapKeys(pAddr))
	}

	// IssuedTo.Person.Photo should be a map.
	photo, ok := person["Photo"].(map[string]any)
	if !ok {
		t.Errorf("Person.Photo: got %T (%v), want map", person["Photo"], person["Photo"])
	} else {
		t.Logf("Person.Photo keys: %v", mapKeys(photo))
	}
}
