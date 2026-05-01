// Copyright 2023 Princess B33f Heavy Industries / Dave Shanley
// SPDX-License-Identifier: MIT
package schema_validation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"

	xj "github.com/basgys/goxml2json"

	liberrors "github.com/pb33f/libopenapi-validator/errors"
	"github.com/pb33f/libopenapi-validator/helpers"
)

func (x *xmlValidator) validateXMLWithVersion(schema *base.Schema, xmlString string, log *slog.Logger, version float32) (bool, []*liberrors.ValidationError) {
	if schema == nil {
		log.Info("schema is empty and cannot be validated")
		return false, nil
	}

	// parse xml and transform to json structure matching schema
	transformedJSON, prevalidationErrors := TransformXMLToSchemaJSON(xmlString, schema)
	if len(prevalidationErrors) > 0 {
		return false, prevalidationErrors
	}

	// validate transformed json against schema using existing validator
	return x.schemaValidator.validateSchemaWithVersion(schema, nil, transformedJSON, log, version)
}

// TransformXMLToSchemaJSON converts xml to json structure matching openapi schema.
// applies xml object transformations: name, attribute, wrapped.
func TransformXMLToSchemaJSON(xmlString string, schema *base.Schema) (any, []*liberrors.ValidationError) {
	if xmlString == "" {
		return nil, []*liberrors.ValidationError{liberrors.InvalidXMLParsing("empty xml content", xmlString)}
	}

	// parse xml using goxml2json. we convert types manually
	jsonBuf, err := xj.Convert(strings.NewReader(xmlString))
	if err != nil {
		return nil, []*liberrors.ValidationError{liberrors.InvalidXMLParsing(fmt.Sprintf("malformed xml: %s", err.Error()), xmlString)}
	}

	jsonBytes := jsonBuf.Bytes()

	// the smallest valid XML possible "<a></a>" generates a 10 bytes buffer.
	// any other invalid XML generates a smaller buffer
	if len(jsonBytes) < 10 {
		return nil, []*liberrors.ValidationError{liberrors.InvalidXMLParsing("malformed xml", xmlString)}
	}

	var rawJSON any
	if err := json.Unmarshal(jsonBytes, &rawJSON); err != nil {
		return nil, []*liberrors.ValidationError{liberrors.InvalidXMLParsing(fmt.Sprintf("failed to decode converted xml to json: %s", err.Error()), xmlString)}
	}

	xmlNsMap := make(map[string]string, 2)

	// apply openapi xml object transformations
	return applyXMLTransformations(rawJSON, schema, &xmlNsMap)
}

func validateXmlNs(dataMap *map[string]any, schema *base.Schema, propName string, xmlNsMap *map[string]string) []*liberrors.ValidationError {
	var validationErrors []*liberrors.ValidationError

	if dataMap == nil || schema == nil || xmlNsMap == nil {
		return validationErrors
	}

	if propName != "" {
		if val, exists := (*dataMap)[propName]; exists {
			if converted, ok := val.(map[string]any); ok {
				dataMap = &converted
			}
		}
	}

	if schema.XML.Prefix != "" {
		attrKey := "-" + schema.XML.Prefix

		val, exists := (*dataMap)[attrKey]

		if exists {
			if ns, ok := val.(string); ok {
				(*xmlNsMap)[schema.XML.Prefix] = ns
				(*xmlNsMap)[ns] = schema.XML.Prefix

				if schema.XML.Namespace != "" && schema.XML.Namespace != ns {
					validationErrors = append(validationErrors,
						liberrors.InvalidNamespace(schema, ns, schema.XML.Namespace, schema.XML.Prefix))
				}
			}

			delete((*dataMap), attrKey)
		} else {
			validationErrors = append(validationErrors, liberrors.MissingPrefix(schema, schema.XML.Prefix))
		}
	}

	if schema.XML.Namespace != "" {
		_, exists := (*xmlNsMap)[schema.XML.Namespace]

		if !exists {
			validationErrors = append(validationErrors, liberrors.MissingNamespace(schema, schema.XML.Namespace))
		}
	}

	return validationErrors
}

// resolveAllOfForXML merges allOf branches into an effective schema for
// XML type coercion. When a property schema is allOf: [{$ref: Type}, {description}],
// the type, items, properties, and XML metadata live on the $ref branch,
// not the top-level schema. This function returns a schema with those
// fields populated so convertBasedOnSchema can coerce values correctly.
func resolveAllOfForXML(schema *base.Schema) *base.Schema {
	visited := make(map[*base.Schema]struct{})
	return resolveAllOfWalk(schema, visited)
}

func resolveAllOfWalk(schema *base.Schema, visited map[*base.Schema]struct{}) *base.Schema {
	if _, seen := visited[schema]; seen {
		return schema
	}
	visited[schema] = struct{}{}

	if len(schema.AllOf) == 0 {
		return schema
	}

	// If the schema already has a type, items, or properties, use it as-is.
	if len(schema.Type) > 0 || schema.Items != nil ||
		(schema.Properties != nil && schema.Properties.Len() > 0) {
		return schema
	}

	// Walk allOf branches and pick up type, items, properties, XML from
	// the first branch that has them.
	for _, proxy := range schema.AllOf {
		branch := proxy.Schema()
		if branch == nil {
			continue
		}

		resolved := resolveAllOfWalk(branch, visited)

		if len(resolved.Type) > 0 || resolved.Items != nil ||
			(resolved.Properties != nil && resolved.Properties.Len() > 0) {
			return resolved
		}
	}

	return schema
}

func convertBasedOnSchema(propName, xmlName string, propValue any, schema *base.Schema, xmlNsMap *map[string]string) (any, []*liberrors.ValidationError) {
	var xmlNsErrors []*liberrors.ValidationError

	// Resolve the effective schema: when the schema is purely an allOf
	// wrapper (no direct type/properties), merge the allOf branches to
	// get the actual type, items, and properties for coercion.
	effective := resolveAllOfForXML(schema)

	types := effective.Type

	extractTypes := func(proxies []*base.SchemaProxy) {
		for _, proxy := range proxies {
			sch := proxy.Schema()
			if len(sch.Type) > 0 {
				types = append(types, sch.Type...)
			}
		}
	}

	extractTypes(effective.AllOf)
	extractTypes(effective.OneOf)
	extractTypes(effective.AnyOf)

	convertedValue := propValue

typesLoop:
	for _, pType := range types {
		// because in XML everything is a string, we try to convert the value to the
		// actual expected type, so the normal schema validation should pass with correct types
		switch pType {
		case helpers.Integer:
			textValue, isString := propValue.(string)

			if isString {
				converted, err := strconv.ParseInt(textValue, 10, 64)

				if err == nil {
					convertedValue = converted
					break typesLoop
				}
			}
		case helpers.Number:
			textValue, isString := propValue.(string)

			if isString {
				converted, err := strconv.ParseFloat(textValue, 64)
				if err == nil {
					convertedValue = converted
					break typesLoop
				}
			}

		case helpers.Boolean:
			textValue, isString := propValue.(string)

			if isString {
				converted, err := strconv.ParseBool(textValue)
				if err == nil {
					convertedValue = converted
					break typesLoop
				}
			}

		case helpers.Array:
			convertedValue = propValue

			if effective.XML != nil && effective.XML.Wrapped {
				convertedValue = unwrapArrayElement(propValue, propName, effective)
			}

			if effective.Items != nil && effective.Items.A != nil {
				itemSchema := effective.Items.A.Schema()

				arr, isArr := convertedValue.([]any)

				if !isArr {
					arr = []any{
						convertedValue,
					}
				}

				for index, item := range arr {
					converted, errs := convertBasedOnSchema(propName, xmlName, item, itemSchema, xmlNsMap)

					if len(errs) > 0 {
						xmlNsErrors = append(xmlNsErrors, errs...)
					}

					arr[index] = converted
				}

				convertedValue = arr
				break typesLoop
			}
		case helpers.Object:
			objectValue, isObject := propValue.(map[string]any)

			if isObject {
				newValue, xmlErrors := applyXMLTransformations(objectValue, effective, xmlNsMap)

				if len(xmlErrors) > 0 {
					xmlNsErrors = append(xmlNsErrors, xmlErrors...)
					continue typesLoop
				}

				convertedValue = newValue
				break typesLoop
			}
		}
	}

	return convertedValue, xmlNsErrors
}

// applyXMLTransformations applies openapi xml object rules to match json schema.
// handles xml.name (root unwrapping), xml.attribute (dash prefix), xml.wrapped (array unwrapping),
// xml.prefix (check existance), xml.namespace (check if exists and match).
// we delete all attributes, prefixes, and namespaces found in the data interface; therefore, undeclared items
// are sent in the body for validation, so that 'additionalProperties: false' can detect it.
func applyXMLTransformations(data any, schema *base.Schema, xmlNsMap *map[string]string) (any, []*liberrors.ValidationError) {
	if schema == nil || data == nil || xmlNsMap == nil {
		return data, nil
	}

	// Resolve allOf wrappers so we can access type, properties, items.
	schema = resolveAllOfForXML(schema)

	// Unwrap root/wrapper elements. The goxml2json output wraps content
	// in an element-named key. We unwrap when:
	// - the schema has an explicit xml.name matching a key, or
	// - there's a single key that is NOT a declared property (it's a wrapper)
	// We do NOT unwrap when the single key IS a declared property name —
	// that's a real nested object (e.g. {"Organization": {...}} inside IssuedBy).
	if dataMap, ok := data.(map[string]any); ok {
		if schema.XML != nil && schema.XML.Name != "" {
			// explicit xml.name — unwrap that specific element
			if wrapped, exists := dataMap[schema.XML.Name]; exists {
				data = wrapped
			}
		} else if len(dataMap) == 1 {
			// Single key — unwrap only if it's NOT a declared property.
			// Also check dash-prefixed keys against xml attribute properties:
			// goxml2json encodes XML attributes as "-attrName", and the attribute
			// rename happens later. Without this check, a single-attribute object
			// like <Photo format="jpg"/> (→ {"-format":"jpg"}) gets unwrapped to
			// the bare string "jpg" before the attribute rename can fire.
			for key, wrapped := range dataMap {
				isDeclaredProp := false
				if schema.Properties != nil {
					trimmed := strings.TrimPrefix(key, "-")
					for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
						if pair.Key() == key || pair.Key() == trimmed {
							isDeclaredProp = true

							break
						}
					}
				}

				if !isDeclaredProp {
					data = wrapped
				}

				break
			}
		}
	}

	// After root unwrapping, apply type coercion for non-object/non-string root
	// schemas. Scalars (integer, number, boolean) need coercion from XML text.
	// Arrays need element-name unwrapping. Strings and objects are already correct.
	if len(schema.Type) > 0 &&
		schema.Type[0] != helpers.Object &&
		schema.Type[0] != helpers.String {
		// For root-level arrays, the root unwrap produces {"elementName": [...]}.
		// Extract the inner array before passing to convertBasedOnSchema.
		if schema.Type[0] == helpers.Array {
			if dataMap, ok := data.(map[string]any); ok && len(dataMap) == 1 {
				for _, v := range dataMap {
					data = v

					break
				}
			}
		}

		converted, errs := convertBasedOnSchema("", "", data, schema, xmlNsMap)
		if len(errs) == 0 {
			data = converted
		}

		return data, errs
	}

	var xmlNsErrors []*liberrors.ValidationError

	// transform properties based on their xml configurations
	if dataMap, ok := data.(map[string]any); ok {
		if schema.Properties == nil || schema.Properties.Len() == 0 {
			if schema.XML != nil && (schema.XML.Prefix != "" || schema.XML.Namespace != "") {
				namespaceErrors := validateXmlNs(&dataMap, schema, "", xmlNsMap)

				if len(namespaceErrors) > 0 {
					xmlNsErrors = append(xmlNsErrors, namespaceErrors...)
				} else {
					if content, has := dataMap["#content"]; has {
						if stringContent, ok := content.(string); ok {
							data = stringContent
						}
					}
				}
			}

			return data, xmlNsErrors
		}

		for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
			propName := pair.Key()
			propSchemaProxy := pair.Value()
			propSchema := propSchemaProxy.Schema()
			if propSchema == nil {
				continue
			}

			xmlName := propName

			if propSchema.XML != nil {
				// determine xml element name (defaults to property name)
				if propSchema.XML.Name != "" {
					xmlName = propSchema.XML.Name
				}
			}

			if propSchema.XML != nil {
				namespaceErrors := validateXmlNs(&dataMap, propSchema, xmlName, xmlNsMap)

				if len(namespaceErrors) > 0 {
					xmlNsErrors = append(xmlNsErrors, namespaceErrors...)
				}

				// handle xml.attribute: true - attributes are prefixed with dash
				if propSchema.XML.Attribute {
					attrKey := "-" + xmlName
					if val, exists := dataMap[attrKey]; exists {
						// If the value is an attribute, it cannot have a namespace
						convertedValue, _ := convertBasedOnSchema(propName, xmlName, val, propSchema, xmlNsMap)
						dataMap[propName] = convertedValue
						delete(dataMap, attrKey)
						continue
					}
				}
			}

			// handle regular elements
			if val, exists := dataMap[xmlName]; exists {
				if mapObject, ok := val.(map[string]any); ok {
					if content, has := mapObject["#content"]; has {
						if stringContent, ok := content.(string); ok {
							val = stringContent
						}
					}
				}

				convertedValue, nsErrors := convertBasedOnSchema(propName, xmlName, val, propSchema, xmlNsMap)

				if len(nsErrors) > 0 {
					xmlNsErrors = append(xmlNsErrors, nsErrors...)
				}

				dataMap[propName] = convertedValue

				if propName != xmlName {
					delete(dataMap, xmlName)
				}
			}
		}
	}

	return data, xmlNsErrors
}

// unwrapArrayElement removes wrapping element from xml arrays when xml.wrapped is true.
// example: {"items": {"item": [...]}} becomes [...]
func unwrapArrayElement(val any, itemName string, propSchema *base.Schema) any {
	wrapMap, ok := val.(map[string]any)
	if !ok {
		return val
	}

	if propSchema.XML.Name != "" {
		itemName = propSchema.XML.Name
	}

	// determine item element name
	if propSchema.Items != nil && propSchema.Items.A != nil {
		itemSchema := propSchema.Items.A.Schema()
		if itemSchema != nil && itemSchema.XML != nil && itemSchema.XML.Name != "" {
			itemName = itemSchema.XML.Name
		}
	}

	// unwrap: look for item element inside wrapper
	if unwrapped, exists := wrapMap[itemName]; exists {
		return unwrapped
	}

	return val
}

// IsXMLContentType checks if a media type string represents xml content.
func IsXMLContentType(mediaType string) bool {
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	return strings.HasPrefix(mt, "application/xml") ||
		strings.HasPrefix(mt, "text/xml") ||
		strings.HasSuffix(mt, "+xml")
}
