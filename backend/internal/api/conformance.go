package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// This file implements the OpenAPI conformance checks for the protected API
// slices. It reads the normative contract at contracts/api/openapi.yaml and:
//
//   - verifies the document is OpenAPI 3.1;
//   - extracts the applicable protected operations (Onboarding, Properties,
//     Documents and Billing tags) with their methods, paths, operation ids and
//     declared response codes;
//   - validates Resource, Collection and Error response envelopes against the
//     contract's component schemas.
//
// The validators are schema-driven: every rule comes from the parsed contract
// rather than from a duplicated in-code copy.

// ProtectedSliceTags are the OpenAPI tags whose operations belong to the
// protected owner, property, onboarding and contract API slice.
var ProtectedSliceTags = []string{"Onboarding", "Properties"}

// FinanceSliceTags are the OpenAPI tags whose operations belong to the
// protected document, billing, approval and reporting API slice.
var FinanceSliceTags = []string{"Documents", "Billing"}

// Operation describes one protected HTTP operation declared in the contract.
type Operation struct {
	Method      string
	Path        string
	OperationID string
	Tag         string
	Responses   []string
}

// Spec is a parsed view of contracts/api/openapi.yaml.
type Spec struct {
	root map[string]any
}

// LoadSpec parses the OpenAPI contract at path.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("openapi: read contract: %w", err)
	}
	root, err := parseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("openapi: parse contract: %w", err)
	}
	rootMap, ok := root.(map[string]any)
	if !ok {
		return nil, errors.New("openapi: contract root is not a mapping")
	}
	spec := &Spec{root: rootMap}
	if !strings.HasPrefix(spec.openAPIVersion(), "3.1") {
		return nil, fmt.Errorf("openapi: contract version %q is not OpenAPI 3.1", spec.openAPIVersion())
	}
	return spec, nil
}

func (s *Spec) openAPIVersion() string {
	v, _ := s.root["openapi"].(string)
	return v
}

// OpenAPIVersion returns the contract's openapi version string.
func (s *Spec) OpenAPIVersion() string {
	return s.openAPIVersion()
}

// Lint validates the OpenAPI contract document and returns a sorted list of
// structural problems found. An empty result means the document lints clean.
// The checks are schema-driven and cover the stability rules that matter for
// the protected API slices: version pinning, resolvable references, unique
// operation ids, declared tags, declared security schemes, valid HTTP methods
// and response codes, and per-operation response declarations.
func (s *Spec) Lint() []string {
	var issues []string

	if !strings.HasPrefix(s.openAPIVersion(), "3.1") {
		issues = append(issues, fmt.Sprintf("openapi version %q is not 3.1", s.openAPIVersion()))
	}

	declaredTags := map[string]bool{}
	if tags, ok := s.root["tags"].([]any); ok {
		for _, t := range tags {
			if tm, ok := t.(map[string]any); ok {
				if name := s.string(tm["name"]); name != "" {
					declaredTags[name] = true
				}
			}
		}
	}

	schemes := s.securitySchemeNames()
	for _, name := range s.referencedSecuritySchemes() {
		if !schemes[name] {
			issues = append(issues, fmt.Sprintf("security scheme %q is referenced but not declared", name))
		}
	}

	for _, ref := range s.allRefs() {
		if !strings.HasPrefix(ref, "#/") {
			issues = append(issues, fmt.Sprintf("non-local reference %q is not allowed", ref))
			continue
		}
		if _, ok := s.resolveRef(ref); !ok {
			issues = append(issues, fmt.Sprintf("reference %q does not resolve", ref))
		}
	}

	seen := map[string]string{}
	paths, _ := s.root["paths"].(map[string]any)
	for path, node := range paths {
		if !strings.HasPrefix(path, "/") {
			issues = append(issues, fmt.Sprintf("path %q must start with /", path))
		}
		methods, ok := node.(map[string]any)
		if !ok {
			issues = append(issues, fmt.Sprintf("path %q is not an object", path))
			continue
		}
		for method, opNode := range methods {
			method = strings.ToLower(method)
			if !isHTTPMethod(method) {
				continue
			}
			op, ok := opNode.(map[string]any)
			if !ok {
				issues = append(issues, fmt.Sprintf("%s %s: operation is not an object", method, path))
				continue
			}
			opID := s.string(op["operationId"])
			if opID == "" {
				issues = append(issues, fmt.Sprintf("%s %s: operation has no operationId", method, path))
			} else if prior, dup := seen[opID]; dup {
				issues = append(issues, fmt.Sprintf("operationId %q is duplicated (%s and %s %s)", opID, prior, method, path))
			} else {
				seen[opID] = method + " " + path
			}
			for _, tag := range s.stringList(op["tags"]) {
				if !declaredTags[tag] {
					issues = append(issues, fmt.Sprintf("%s %s: tag %q is not declared", method, path, tag))
				}
			}
			for _, code := range s.responseCodes(op["responses"]) {
				if code == "default" {
					continue
				}
				if !validStatus(code) {
					issues = append(issues, fmt.Sprintf("%s %s: invalid response code %q", method, path, code))
				}
			}
			if resp, ok := op["responses"].(map[string]any); ok && len(resp) == 0 {
				issues = append(issues, fmt.Sprintf("%s %s: operation declares no responses", method, path))
			}
		}
	}

	sort.Strings(issues)
	return issues
}

func isHTTPMethod(method string) bool {
	switch method {
	case "get", "put", "post", "delete", "patch", "head", "options", "trace":
		return true
	}
	return false
}

func validStatus(code string) bool {
	n, err := strconv.Atoi(code)
	if err != nil {
		return false
	}
	return n >= 100 && n <= 599
}

func (s *Spec) securitySchemeNames() map[string]bool {
	out := map[string]bool{}
	components, _ := s.root["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	for name := range schemes {
		out[name] = true
	}
	return out
}

// referencedSecuritySchemes collects every scheme name referenced by the root
// security list and any operation-level security list.
func (s *Spec) referencedSecuritySchemes() []string {
	var out []string
	collect := func(node any) {
		list, ok := node.([]any)
		if !ok {
			return
		}
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for name := range m {
				out = append(out, name)
			}
		}
	}
	collect(s.root["security"])
	if paths, ok := s.root["paths"].(map[string]any); ok {
		for _, node := range paths {
			methods, ok := node.(map[string]any)
			if !ok {
				continue
			}
			for _, opNode := range methods {
				op, ok := opNode.(map[string]any)
				if !ok {
					continue
				}
				collect(op["security"])
			}
		}
	}
	return out
}

// allRefs walks the document and returns every $ref value found.
func (s *Spec) allRefs() []string {
	var out []string
	var walk func(any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			for key, val := range v {
				if key == "$ref" {
					if ref, ok := val.(string); ok {
						out = append(out, ref)
					}
					continue
				}
				walk(val)
			}
		case []any:
			for _, el := range v {
				walk(el)
			}
		}
	}
	walk(s.root)
	return out
}

// ProtectedOperations returns the contract operations tagged Onboarding or
// Properties, sorted deterministically by method then path.
func (s *Spec) ProtectedOperations() ([]Operation, error) {
	return s.OperationsForTags(ProtectedSliceTags)
}

// FinanceOperations returns the contract operations tagged Documents or
// Billing (the protected document, billing, approval and reporting slice),
// sorted deterministically by method then path.
func (s *Spec) FinanceOperations() ([]Operation, error) {
	return s.OperationsForTags(FinanceSliceTags)
}

// AllOperations returns every operation declared in the contract across all
// tags, sorted deterministically by method then path.
func (s *Spec) AllOperations() ([]Operation, error) {
	paths, ok := s.root["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("openapi: contract has no paths mapping")
	}
	var ops []Operation
	for path, node := range paths {
		methods, ok := node.(map[string]any)
		if !ok {
			continue
		}
		for method, opNode := range methods {
			op, ok := opNode.(map[string]any)
			if !ok {
				continue
			}
			ops = append(ops, Operation{
				Method:      strings.ToUpper(method),
				Path:        path,
				OperationID: s.string(op["operationId"]),
				Tag:         firstTag(s.stringList(op["tags"])),
				Responses:   s.responseCodes(op["responses"]),
			})
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Method != ops[j].Method {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})
	return ops, nil
}

// AllTags returns every tag name declared at the document level.
func (s *Spec) AllTags() []string {
	tags, ok := s.root["tags"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, t := range tags {
		if tm, ok := t.(map[string]any); ok {
			if name := s.string(tm["name"]); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// OperationSecurity returns the security requirements that apply to an
// operation. The result is the operation-level security override when present,
// otherwise the document-global security.
func (s *Spec) OperationSecurity(method, path string) []string {
	paths, ok := s.root["paths"].(map[string]any)
	if !ok {
		return nil
	}
	pathNode, ok := paths[path].(map[string]any)
	if !ok {
		return nil
	}
	opNode, ok := pathNode[strings.ToLower(method)].(map[string]any)
	if !ok {
		return nil
	}
	sec, ok := opNode["security"]
	if !ok || sec == nil {
		sec = s.root["security"]
	}
	list, ok := sec.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		for name := range m {
			out = append(out, name)
		}
	}
	return out
}

// IdempotencyRoutes returns paths whose request body schema defines an
// idempotency_key property, i.e. the contract requires idempotency on those
// mutations.
func (s *Spec) IdempotencyRoutes() map[string]bool {
	out := map[string]bool{}
	paths, ok := s.root["paths"].(map[string]any)
	if !ok {
		return out
	}
	for path, pathNode := range paths {
		methods, ok := pathNode.(map[string]any)
		if !ok {
			continue
		}
		for method, opNode := range methods {
			op, ok := opNode.(map[string]any)
			if !ok {
				continue
			}
			if hasRequestBodyIdempotencyKey(s, op) {
				out[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	return out
}

func hasRequestBodyIdempotencyKey(s *Spec, op map[string]any) bool {
	rb, ok := op["requestBody"].(map[string]any)
	if !ok {
		return false
	}
	rb = resolveSchemaRef(s, rb)
	if rb == nil {
		return false
	}
	content, ok := rb["content"].(map[string]any)
	if !ok {
		return false
	}
	jsonMT, ok := content["application/json"].(map[string]any)
	if !ok {
		return false
	}
	schemaNode := jsonMT["schema"]
	schema := resolveSchemaRef(s, schemaNode)
	if schema == nil {
		return false
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		return false
	}
	_, has := props["idempotency_key"]
	return has
}

func resolveSchemaRef(s *Spec, node any) map[string]any {
	if node == nil {
		return nil
	}
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	if ref, ok := m["$ref"].(string); ok {
		resolved, _ := s.resolveRef(ref)
		return resolved
	}
	return m
}

func firstTag(tags []string) string {
	if len(tags) > 0 {
		return tags[0]
	}
	return ""
}

// OperationsForTags returns the contract operations tagged with any of the
// given tags, sorted deterministically by method then path.
func (s *Spec) OperationsForTags(tags []string) ([]Operation, error) {
	paths, ok := s.root["paths"].(map[string]any)
	if !ok {
		return nil, errors.New("openapi: contract has no paths mapping")
	}
	want := map[string]bool{}
	for _, tag := range tags {
		want[tag] = true
	}
	var ops []Operation
	for path, node := range paths {
		methods, ok := node.(map[string]any)
		if !ok {
			continue
		}
		for method, opNode := range methods {
			op, ok := opNode.(map[string]any)
			if !ok {
				continue
			}
			matched := ""
			for _, tag := range s.stringList(op["tags"]) {
				if want[tag] {
					matched = tag
					break
				}
			}
			if matched == "" {
				continue
			}
			ops = append(ops, Operation{
				Method:      strings.ToUpper(method),
				Path:        path,
				OperationID: s.string(op["operationId"]),
				Tag:         matched,
				Responses:   s.responseCodes(op["responses"]),
			})
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Method != ops[j].Method {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})
	return ops, nil
}

func (s *Spec) responseCodes(node any) []string {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	for code := range m {
		if code == "default" {
			continue
		}
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func (s *Spec) string(v any) string {
	str, _ := v.(string)
	return str
}

func (s *Spec) stringList(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, el := range arr {
		if str, ok := el.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

// ValidateResource checks a JSON Resource envelope against the contract's
// Resource schema.
func (s *Spec) ValidateResource(body []byte) error {
	return s.validateEnvelope("Resource", body)
}

// ValidateCollection checks a JSON Collection envelope against the contract's
// Collection schema.
func (s *Spec) ValidateCollection(body []byte) error {
	return s.validateEnvelope("Collection", body)
}

// ValidateError checks a JSON Error envelope against the contract's Error
// schema.
func (s *Spec) ValidateError(body []byte) error {
	return s.validateEnvelope("Error", body)
}

func (s *Spec) validateEnvelope(schemaName string, body []byte) error {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return fmt.Errorf("conformance: %s envelope is not valid JSON: %w", schemaName, err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("conformance: %s envelope must be a JSON object", schemaName)
	}
	schemaNode, ok := s.schemaNode(schemaName)
	if !ok {
		return fmt.Errorf("conformance: contract has no schema %q", schemaName)
	}
	return s.validateSchemaValue(schemaName, obj, schemaNode, schemaName)
}

func (s *Spec) schemaNode(name string) (map[string]any, bool) {
	components, ok := s.root["components"].(map[string]any)
	if !ok {
		return nil, false
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil, false
	}
	node, ok := schemas[name].(map[string]any)
	return node, ok
}

func (s *Spec) resolveRef(ref string) (map[string]any, bool) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	var current any = s.root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[part]
		if !ok {
			return nil, false
		}
		current = next
	}
	node, ok := current.(map[string]any)
	return node, ok
}

func (s *Spec) validateSchemaValue(name string, value any, schema any, path string) error {
	if schema == nil {
		return nil
	}
	if schemaMap, ok := schema.(map[string]any); ok {
		if ref, ok := schemaMap["$ref"].(string); ok {
			resolved, ok := s.resolveRef(ref)
			if !ok {
				return fmt.Errorf("conformance: contract cannot resolve %s", ref)
			}
			return s.validateSchemaValue(name, value, resolved, path)
		}
	}
	// Values are not contract-checked further when the schema is not a map or
	// declares no type constraint (contract leaves the value open).
	if _, ok := schema.(map[string]any); !ok {
		return nil
	}
	types, err := typeNames(schema)
	if err != nil {
		return fmt.Errorf("conformance: %s: %w", path, err)
	}
	if len(types) == 0 {
		return nil
	}
	for _, t := range types {
		if matchesType(value, t) {
			return s.applyConstraints(name, value, t, schema.(map[string]any), path)
		}
	}
	return fmt.Errorf("conformance: %s: field %q expected type %v, got %T", path, name, types, value)
}

func typeNames(schema any) ([]string, error) {
	m, ok := schema.(map[string]any)
	if !ok {
		return nil, nil
	}
	raw, present := m["type"]
	if !present {
		return nil, nil
	}
	switch t := raw.(type) {
	case string:
		return []string{t}, nil
	case []any:
		var out []string
		for _, el := range t {
			str, ok := el.(string)
			if !ok {
				return nil, fmt.Errorf("invalid union type entry %v", el)
			}
			out = append(out, str)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid type constraint %v", raw)
	}
}

func matchesType(value any, t string) bool {
	switch t {
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == math.Trunc(f)
	case "number":
		_, ok := value.(float64)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func (s *Spec) applyConstraints(name string, value any, t string, schema map[string]any, path string) error {
	switch t {
	case "string":
		str := value.(string)
		if minLen, ok := intOf(schema["minLength"]); ok && len(str) < minLen {
			return fmt.Errorf("conformance: %s: field %q shorter than minLength %d", path, name, minLen)
		}
		if maxLen, ok := intOf(schema["maxLength"]); ok && len(str) > maxLen {
			return fmt.Errorf("conformance: %s: field %q longer than maxLength %d", path, name, maxLen)
		}
		if pat, ok := schema["pattern"].(string); ok && pat != "" {
			re, err := regexp.Compile(pat)
			if err == nil && !re.MatchString(str) {
				return fmt.Errorf("conformance: %s: field %q does not match pattern %s", path, name, pat)
			}
		}
	case "integer", "number":
		f := value.(float64)
		if min, ok := numOf(schema["minimum"]); ok && f < min {
			return fmt.Errorf("conformance: %s: field %q below minimum %v", path, name, min)
		}
		if max, ok := numOf(schema["maximum"]); ok && f > max {
			return fmt.Errorf("conformance: %s: field %q above maximum %v", path, name, max)
		}
	case "array":
		arr := value.([]any)
		items := schema["items"]
		if items == nil {
			return nil
		}
		for i, el := range arr {
			if err := s.validateSchemaValue(name, el, items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "object":
		return s.validateObjectFields(name, value.(map[string]any), schema, path)
	}
	return nil
}

func (s *Spec) validateObjectFields(name string, obj map[string]any, schema map[string]any, path string) error {
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			key, _ := r.(string)
			if _, present := obj[key]; !present {
				return fmt.Errorf("conformance: %s: missing required field %q", path, key)
			}
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for key, val := range obj {
		if props != nil {
			if propSchema, ok := props[key]; ok {
				if err := s.validateSchemaValue(key, val, propSchema, path+"."+key); err != nil {
					return err
				}
				continue
			}
		}
		switch additional := schema["additionalProperties"].(type) {
		case bool:
			if !additional {
				return fmt.Errorf("conformance: %s: unexpected field %q (additionalProperties is false)", path, key)
			}
		case map[string]any:
			if err := s.validateSchemaValue(key, val, additional, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func intOf(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	}
	return 0, false
}

func numOf(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}
