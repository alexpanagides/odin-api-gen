package gen

// Intermediate representation: everything the Odin emitter needs, fully
// resolved and named, with no knowledge of OpenAPI left.

type Package struct {
	Name   string
	Models []*Model
	Enums  []*EnumSet
	Groups []*Group // one generated file per tag
}

type Model struct {
	Name   string // Odin type name, Ada_Case
	Doc    []string
	Fields []Field
}

type Field struct {
	OdinName  string // snake_case, keyword-escaped
	WireName  string // JSON property name
	OdinType  string
	Doc       []string
	OmitEmpty bool // request-body optionals: Maybe(T) + omitempty
}

// EnumSet becomes a group of string constants, e.g.
// Transaction_Type_Debit :: "debit"
type EnumSet struct {
	Prefix string // e.g. "Transaction_Type"
	Doc    []string
	Values []EnumValue
}

type EnumValue struct {
	Name  string // full constant name
	Value string
}

type Group struct {
	Tag      string
	FileName string // e.g. api_transaction_accounts.odin
	Ops      []*Operation
}

// ResultKind describes how an operation's success response is decoded.
type ResultKind int

const (
	ResultNone  ResultKind = iota // 204, no payload
	ResultTyped                   // decode into ResultType
	ResultRaw                     // oneOf etc: return json.Value
)

type Operation struct {
	ProcName   string
	Doc        []string
	Method     string // GET, POST, ...
	PathFormat string // Odin fmt.tprintf format, e.g. "/users/%v/transactions"
	PathArgs   []Param
	// Required query params become positional proc args.
	RequiredQuery []Param
	// Optional query params live in an options struct.
	Options     *Model // nil if no optional query params
	OptQuery    []Param
	BodyType    string // "" if no request body
	ResultKind  ResultKind
	ResultType  string // Odin type when ResultKind == ResultTyped
	SuccessCode int
}

type Param struct {
	OdinName string
	WireName string
	OdinType string // string | i64 | f64 | bool
	Doc      []string
}
