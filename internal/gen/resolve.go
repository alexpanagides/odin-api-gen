package gen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Resolve walks a loaded OpenAPI document and produces the Odin IR.
// It supports the subset of OpenAPI this generator is built for and fails
// loudly (with the offending location) on anything else, rather than
// emitting plausible-looking but wrong code.
func Resolve(doc *openapi3.T, packageName string) (*Package, error) {
	r := &resolver{
		doc: doc,
		pkg: &Package{Name: packageName},
	}
	if err := r.operations(); err != nil {
		return nil, err
	}
	return r.pkg, nil
}

// Names the static runtime (client.odin, transport_curl.odin) already takes.
var reservedNames = map[string]bool{
	"Client": true, "Error": true, "Http_Request": true, "Http_Response": true,
	"Transport_Proc": true, "Transport_Error": true, "Api_Error": true,
	"Encode_Error": true, "Decode_Error": true, "Default_Base_Url": true,
}

type resolver struct {
	doc        *openapi3.T
	pkg        *Package
	modelNames map[string]bool
	components map[string]string // component schema name -> emitted model name
	enumSeen   map[string]bool
}

func (r *resolver) addModel(m *Model) error {
	if r.modelNames == nil {
		r.modelNames = map[string]bool{}
	}
	if r.modelNames[m.Name] {
		return fmt.Errorf("duplicate model name %q — add a name override", m.Name)
	}
	if reservedNames[m.Name] {
		return fmt.Errorf("model name %q collides with the SDK runtime", m.Name)
	}
	r.modelNames[m.Name] = true
	r.pkg.Models = append(r.pkg.Models, m)
	return nil
}

// componentModel emits a component schema as a model on first use and
// memoizes it, so only schemas reachable from operations are generated.
// The name is registered before building fields to allow self-reference
// (e.g. Category.children: []Category).
func (r *resolver) componentModel(name string) (string, error) {
	if r.components == nil {
		r.components = map[string]string{}
	}
	if n, ok := r.components[name]; ok {
		return n, nil
	}
	ref, ok := r.doc.Components.Schemas[name]
	if !ok {
		return "", fmt.Errorf("unresolved schema reference %q", name)
	}
	where := "components.schemas." + name
	if !ref.Value.Type.Is("object") {
		return "", fmt.Errorf("%s: only object component schemas are supported", where)
	}
	odinName := AdaCase(name)
	r.components[name] = odinName
	return r.buildModel(odinName, ref.Value, docLines(ref.Value.Description), false, where)
}

// buildModel creates a Model (and any nested synthesized models) from an
// object schema. requiredSet applies request-body optionality rules when
// forRequest is true; response models use plain fields (absent -> zero value).
func (r *resolver) buildModel(name string, s *openapi3.Schema, doc []string, forRequest bool, where string) (string, error) {
	if err := r.checkSupported(s, where); err != nil {
		return "", err
	}
	required := map[string]bool{}
	for _, p := range s.Required {
		required[p] = true
	}
	m := &Model{Name: name, Doc: doc}

	props := make([]string, 0, len(s.Properties))
	for p := range s.Properties {
		props = append(props, p)
	}
	sort.Strings(props)

	for _, prop := range props {
		pref := s.Properties[prop]
		pwhere := where + "." + prop
		typ, err := r.odinType(pref, name+"_"+AdaCase(prop), forRequest, pwhere)
		if err != nil {
			return "", err
		}
		f := Field{
			OdinName: SnakeCase(prop),
			WireName: prop,
			OdinType: typ,
			Doc:      docLines(pref.Value.Description),
		}
		optional := forRequest && !required[prop]
		if pref.Value.Nullable || optional {
			f.OdinType = "Maybe(" + typ + ")"
		}
		f.OmitEmpty = optional
		r.collectEnum(name, prop, pref.Value)
		m.Fields = append(m.Fields, f)
	}
	if err := r.addModel(m); err != nil {
		return "", err
	}
	return name, nil
}

// odinType maps a schema to an Odin type expression, synthesizing named
// models for inline objects (using nameHint).
func (r *resolver) odinType(ref *openapi3.SchemaRef, nameHint string, forRequest bool, where string) (string, error) {
	s := ref.Value
	// A $ref to a component schema keeps its name after resolution.
	if n := refName(ref.Ref); n != "" {
		return r.componentModel(n)
	}
	if err := r.checkSupported(s, where); err != nil {
		return "", err
	}
	switch {
	case s.Type.Is("string"):
		return "string", nil
	case s.Type.Is("integer"):
		return "i64", nil
	case s.Type.Is("number"):
		return "f64", nil
	case s.Type.Is("boolean"):
		return "bool", nil
	case s.Type.Is("array"):
		if s.Items == nil {
			return "", fmt.Errorf("%s: array without items", where)
		}
		if s.Items.Value.Nullable {
			return "", fmt.Errorf("%s: nullable array items are not supported", where)
		}
		elem, err := r.odinType(s.Items, nameHint+"_Item", forRequest, where+".items")
		if err != nil {
			return "", err
		}
		return "[]" + elem, nil
	case s.Type.Is("object") || (s.Type.IsEmpty() && len(s.Properties) > 0):
		if len(s.Properties) == 0 {
			return "", fmt.Errorf("%s: object without properties (free-form) is not supported", where)
		}
		return r.buildModel(nameHint, s, docLines(s.Description), forRequest, where)
	default:
		return "", fmt.Errorf("%s: unsupported schema type %v", where, s.Type.Slice())
	}
}

// checkSupported rejects OpenAPI features outside the generator's subset.
func (r *resolver) checkSupported(s *openapi3.Schema, where string) error {
	switch {
	case len(s.OneOf) > 0:
		return fmt.Errorf("%s: oneOf is only supported at the top level of a response (returned as json.Value)", where)
	case len(s.AnyOf) > 0:
		return fmt.Errorf("%s: anyOf is not supported", where)
	case len(s.AllOf) > 0:
		return fmt.Errorf("%s: allOf is not supported", where)
	case s.Discriminator != nil:
		return fmt.Errorf("%s: discriminator is not supported", where)
	case s.AdditionalProperties.Schema != nil,
		s.AdditionalProperties.Has != nil && *s.AdditionalProperties.Has:
		return fmt.Errorf("%s: additionalProperties is not supported", where)
	}
	return nil
}

// collectEnum records string-enum properties of named models as constant sets.
func (r *resolver) collectEnum(modelName, prop string, s *openapi3.Schema) {
	if len(s.Enum) == 0 || !s.Type.Is("string") {
		return
	}
	prefix := modelName + "_" + AdaCase(prop)
	if r.enumSeen == nil {
		r.enumSeen = map[string]bool{}
	}
	if r.enumSeen[prefix] {
		return
	}
	r.enumSeen[prefix] = true
	es := &EnumSet{
		Prefix: prefix,
		Doc:    []string{fmt.Sprintf("Allowed values for %s.%s.", modelName, SnakeCase(prop))},
	}
	for _, v := range s.Enum {
		sv, ok := v.(string)
		if !ok {
			continue // enum lists may contain null for nullable fields
		}
		es.Values = append(es.Values, EnumValue{
			Name:  prefix + "_" + AdaCase(sv),
			Value: sv,
		})
	}
	if len(es.Values) > 0 {
		r.pkg.Enums = append(r.pkg.Enums, es)
	}
}

// --- paths -> operations ---

var methodOrder = []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

func (r *resolver) operations() error {
	groups := map[string]*Group{}

	paths := r.doc.Paths.Map()
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, path := range pathKeys {
		item := paths[path]
		ops := item.Operations()
		for _, method := range methodOrder {
			op, ok := ops[method]
			if !ok {
				continue
			}
			where := method + " " + path
			irop, tag, err := r.operation(method, path, item, op, where)
			if err != nil {
				return err
			}
			g, ok := groups[tag]
			if !ok {
				g = &Group{Tag: tag, FileName: "api_" + SnakeCase(tag) + ".odin"}
				groups[tag] = g
			}
			g.Ops = append(g.Ops, irop)
		}
	}

	tags := make([]string, 0, len(groups))
	for t := range groups {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	seenProcs := map[string]bool{}
	for _, t := range tags {
		r.pkg.Groups = append(r.pkg.Groups, groups[t])
		for _, op := range groups[t].Ops {
			if seenProcs[op.ProcName] {
				return fmt.Errorf("duplicate proc name %q derived from summaries", op.ProcName)
			}
			seenProcs[op.ProcName] = true
		}
	}
	return nil
}

func (r *resolver) operation(method, path string, item *openapi3.PathItem, op *openapi3.Operation, where string) (*Operation, string, error) {
	if op.Summary == "" {
		return nil, "", fmt.Errorf("%s: operation has no operationId and no summary to derive a name from", where)
	}
	tag := "default"
	if len(op.Tags) > 0 {
		tag = op.Tags[0]
	}
	o := &Operation{
		ProcName: ProcName(op.Summary),
		Method:   method,
		Doc:      opDoc(op),
	}

	// Parameters: merge path-item level and operation level.
	params := append(append([]*openapi3.ParameterRef{}, item.Parameters...), op.Parameters...)
	byName := map[[2]string]*openapi3.Parameter{}
	for _, pr := range params {
		p := pr.Value
		byName[[2]string{p.In, p.Name}] = p
	}

	// Path params, in order of appearance in the path template.
	format := path
	for _, seg := range pathTemplateVars(path) {
		p, ok := byName[[2]string{"path", seg}]
		if !ok {
			return nil, "", fmt.Errorf("%s: path variable {%s} has no parameter definition", where, seg)
		}
		prm, err := r.param(p, where)
		if err != nil {
			return nil, "", err
		}
		o.PathArgs = append(o.PathArgs, prm)
		format = strings.Replace(format, "{"+seg+"}", "%v", 1)
	}
	o.PathFormat = format

	// Query params: required -> positional args, optional -> options struct.
	seen := map[[2]string]bool{}
	for _, pr := range params {
		p := pr.Value
		if seen[[2]string{p.In, p.Name}] {
			continue
		}
		seen[[2]string{p.In, p.Name}] = true
		switch p.In {
		case "path":
			// handled above
		case "query":
			prm, err := r.param(p, where)
			if err != nil {
				return nil, "", err
			}
			if p.Required {
				o.RequiredQuery = append(o.RequiredQuery, prm)
			} else {
				o.OptQuery = append(o.OptQuery, prm)
			}
		default:
			return nil, "", fmt.Errorf("%s: %s parameters are not supported", where, p.In)
		}
	}
	if len(o.OptQuery) > 0 {
		m := &Model{
			Name: AdaCase(o.ProcName) + "_Options",
			Doc:  []string{fmt.Sprintf("Optional query parameters for %s.", o.ProcName)},
		}
		for _, q := range o.OptQuery {
			m.Fields = append(m.Fields, Field{
				OdinName: q.OdinName,
				WireName: q.WireName,
				OdinType: "Maybe(" + q.OdinType + ")",
				Doc:      q.Doc,
			})
		}
		if err := r.addModel(m); err != nil {
			return nil, "", err
		}
		o.Options = m
	}

	// Request body.
	if op.RequestBody != nil {
		rb := op.RequestBody.Value
		schema, err := jsonSchema(rb.Content, where+" requestBody")
		if err != nil {
			return nil, "", err
		}
		name := AdaCase(o.ProcName) + "_Body"
		if n := refName(schema.Ref); n != "" {
			name, err = r.componentModel(n)
			if err != nil {
				return nil, "", err
			}
		} else {
			if !schema.Value.Type.Is("object") {
				return nil, "", fmt.Errorf("%s: only object request bodies are supported", where)
			}
			if _, err := r.buildModel(name, schema.Value, []string{fmt.Sprintf("Request body for %s.", o.ProcName)}, true, where+" requestBody"); err != nil {
				return nil, "", err
			}
		}
		o.BodyType = name
	}

	// Success response: exactly one 2xx.
	if err := r.response(o, op, where); err != nil {
		return nil, "", err
	}
	return o, tag, nil
}

func (r *resolver) response(o *Operation, op *openapi3.Operation, where string) error {
	var codes []string
	respMap := op.Responses.Map()
	for code := range respMap {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	if len(codes) != 1 {
		return fmt.Errorf("%s: expected exactly one 2xx response, got %v", where, codes)
	}
	code := codes[0]
	fmt.Sscanf(code, "%d", &o.SuccessCode)
	resp := respMap[code].Value

	if len(resp.Content) == 0 {
		o.ResultKind = ResultNone
		return nil
	}
	schema, err := jsonSchema(resp.Content, where+" response "+code)
	if err != nil {
		return err
	}
	s := schema.Value
	rwhere := where + " response " + code

	if len(s.OneOf) > 0 {
		// Bounded support: a oneOf response is returned as parsed json.Value.
		o.ResultKind = ResultRaw
		return nil
	}
	typ, err := r.odinType(schema, AdaCase(o.ProcName)+"_Response", false, rwhere)
	if err != nil {
		return err
	}
	o.ResultKind = ResultTyped
	o.ResultType = typ
	return nil
}

func (r *resolver) param(p *openapi3.Parameter, where string) (Param, error) {
	pwhere := fmt.Sprintf("%s param %q", where, p.Name)
	if p.Schema == nil {
		return Param{}, fmt.Errorf("%s: parameter without schema", pwhere)
	}
	// Only default style/explode (simple path, form query) is supported.
	if p.Style != "" && p.Style != "simple" && p.Style != "form" {
		return Param{}, fmt.Errorf("%s: parameter style %q is not supported", pwhere, p.Style)
	}
	s := p.Schema.Value
	var typ string
	switch {
	case s.Type.Is("string"):
		typ = "string"
	case s.Type.Is("integer"):
		typ = "i64"
	case s.Type.Is("number"):
		typ = "f64"
	case s.Type.Is("boolean"):
		typ = "bool"
	default:
		return Param{}, fmt.Errorf("%s: unsupported parameter type %v", pwhere, s.Type.Slice())
	}
	doc := docLines(p.Description)
	if len(s.Enum) > 0 {
		var vals []string
		for _, v := range s.Enum {
			if sv, ok := v.(string); ok {
				vals = append(vals, fmt.Sprintf("%q", sv))
			}
		}
		doc = append(doc, "Allowed values: "+strings.Join(vals, ", ")+".")
	}
	return Param{
		OdinName: SnakeCase(p.Name),
		WireName: p.Name,
		OdinType: typ,
		Doc:      doc,
	}, nil
}

// --- helpers ---

func refName(ref string) string {
	const p = "#/components/schemas/"
	if strings.HasPrefix(ref, p) {
		return strings.TrimPrefix(ref, p)
	}
	return ""
}

func jsonSchema(content openapi3.Content, where string) (*openapi3.SchemaRef, error) {
	mt := content.Get("application/json")
	if mt == nil || mt.Schema == nil {
		types := make([]string, 0, len(content))
		for t := range content {
			types = append(types, t)
		}
		return nil, fmt.Errorf("%s: only application/json content is supported, got %v", where, types)
	}
	return mt.Schema, nil
}

func pathTemplateVars(path string) []string {
	var vars []string
	rest := path
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			return vars
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return vars
		}
		vars = append(vars, rest[i+1:i+j])
		rest = rest[i+j+1:]
	}
}

func docLines(desc string) []string {
	desc = strings.TrimSpace(strings.ReplaceAll(desc, "\r\n", "\n"))
	if desc == "" {
		return nil
	}
	return strings.Split(desc, "\n")
}

func opDoc(op *openapi3.Operation) []string {
	lines := docLines(op.Summary)
	if d := docLines(op.Description); len(d) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, d...)
	}
	return lines
}
