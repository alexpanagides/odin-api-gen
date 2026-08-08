// Static runtime for the generated SDK. Written by hand, copied verbatim
// by odin-api-gen (only the package name and base URL are substituted).
package PACKAGE_NAME

import "base:runtime"
import "core:encoding/json"
import "core:fmt"
import "core:strings"

DEFAULT_BASE_URL :: "BASE_URL_PLACEHOLDER"

Client :: struct {
	// Base URL of the API without a trailing slash. Empty uses DEFAULT_BASE_URL.
	base_url:      string,
	// Personal developer key, sent as X-Developer-Key.
	developer_key: string,
	// OAuth 2 bearer access token; used when developer_key is empty.
	access_token:  string,
	// HTTP transport; nil uses the default (libcurl-backed) transport.
	transport:      Transport_Proc,
	transport_data: rawptr,
}

// client_from_developer_key creates a client authenticated with a personal
// developer key.
client_from_developer_key :: proc(key: string) -> Client {
	return Client{developer_key = key}
}

// client_from_oauth_token creates a client authenticated with an OAuth 2
// bearer access token.
client_from_oauth_token :: proc(token: string) -> Client {
	return Client{access_token = token}
}

Http_Request :: struct {
	method:  string,
	url:     string,
	headers: [][2]string, // {name, value} pairs
	body:    []byte,      // nil when the request has no body
}

Http_Response :: struct {
	status: int,
	// Allocated with the allocator passed to the transport; the caller frees it.
	body:   []byte,
}

// Transport_Proc performs one HTTP request. Implementations allocate the
// response body with `allocator`. Request strings are only valid for the
// duration of the call.
Transport_Proc :: #type proc(data: rawptr, req: Http_Request, allocator: runtime.Allocator) -> (Http_Response, Maybe(Transport_Error))

// The transport failed before an HTTP status was obtained.
Transport_Error :: struct {
	code:    int,
	message: string,
}

// The server answered with a non-2xx status. `message` is the server's
// error description, allocated with the call's allocator.
Api_Error :: struct {
	status:  int,
	message: string,
}

Encode_Error :: struct {
	message: string,
}

Decode_Error :: struct {
	message: string,
}

Error :: union {
	Transport_Error,
	Api_Error,
	Encode_Error,
	Decode_Error,
}

// _execute builds the URL and headers, runs the transport, and turns non-2xx
// statuses into Api_Error. Scratch data lives on context.temp_allocator; the
// returned body is allocated with `allocator` and owned by the caller.
@(private)
_execute :: proc(c: ^Client, method, path, query: string, body: []byte, allocator: runtime.Allocator) -> (resp: Http_Response, err: Error) {
	sb := strings.builder_make(context.temp_allocator)
	strings.write_string(&sb, c.base_url != "" ? c.base_url : DEFAULT_BASE_URL)
	strings.write_string(&sb, path)
	if query != "" {
		strings.write_byte(&sb, '?')
		strings.write_string(&sb, query)
	}

	headers := make([dynamic][2]string, context.temp_allocator)
	append(&headers, [2]string{"Accept", "application/json"})
	append(&headers, [2]string{"User-Agent", "odin-api-gen-sdk"})
	if body != nil {
		append(&headers, [2]string{"Content-Type", "application/json"})
	}
	if c.developer_key != "" {
		append(&headers, [2]string{"X-Developer-Key", c.developer_key})
	} else if c.access_token != "" {
		append(&headers, [2]string{"Authorization", fmt.tprintf("Bearer %s", c.access_token)})
	}

	transport := c.transport
	if transport == nil {
		transport = _default_transport
	}
	req := Http_Request{
		method  = method,
		url     = strings.to_string(sb),
		headers = headers[:],
		body    = body,
	}
	r, terr := transport(c.transport_data, req, allocator)
	if e, has := terr.?; has {
		err = e
		return
	}
	if r.status < 200 || r.status > 299 {
		err = _api_error(r, allocator)
		delete(r.body, allocator)
		return
	}
	resp = r
	return
}

@(private)
_api_error :: proc(r: Http_Response, allocator: runtime.Allocator) -> Api_Error {
	// The API reports errors as {"error": "..."}; fall back to the raw body.
	Error_Body :: struct {
		error: string,
	}
	e: Error_Body
	if json.unmarshal(r.body, &e, json.DEFAULT_SPECIFICATION, context.temp_allocator) == nil && e.error != "" {
		return Api_Error{status = r.status, message = strings.clone(e.error, allocator)}
	}
	return Api_Error{status = r.status, message = strings.clone(string(r.body), allocator)}
}

@(private)
_encode :: proc(v: any, allocator: runtime.Allocator) -> (data: []byte, err: Error) {
	d, merr := json.marshal(v, {}, context.temp_allocator)
	if merr != nil {
		err = Encode_Error{message = fmt.aprintf("%v", merr, allocator = allocator)}
		return
	}
	data = d
	return
}

@(private)
_decode :: proc($T: typeid, data: []byte, allocator: runtime.Allocator) -> (result: T, err: Error) {
	uerr := json.unmarshal(data, &result, json.DEFAULT_SPECIFICATION, allocator)
	if uerr != nil {
		err = Decode_Error{message = fmt.aprintf("%v", uerr, allocator = allocator)}
	}
	return
}

@(private)
_decode_value :: proc(data: []byte, allocator: runtime.Allocator) -> (result: json.Value, err: Error) {
	v, perr := json.parse(data, json.DEFAULT_SPECIFICATION, false, allocator)
	if perr != .None {
		err = Decode_Error{message = fmt.aprintf("json parse error: %v", perr, allocator = allocator)}
		return
	}
	result = v
	return
}

// --- query string building (scratch data on context.temp_allocator) ---

@(private)
_Query_Builder :: struct {
	sb: strings.Builder,
}

@(private)
_qb_init :: proc(qb: ^_Query_Builder) {
	qb.sb = strings.builder_make(context.temp_allocator)
}

@(private)
_qb_raw :: proc(qb: ^_Query_Builder, key, value: string) {
	if strings.builder_len(qb.sb) > 0 {
		strings.write_byte(&qb.sb, '&')
	}
	strings.write_string(&qb.sb, key) // keys come from the spec, never escaped
	strings.write_byte(&qb.sb, '=')
	strings.write_string(&qb.sb, _percent_encode(value))
}

@(private)
_qb_add_string :: proc(qb: ^_Query_Builder, key: string, v: string) {
	_qb_raw(qb, key, v)
}

@(private)
_qb_add_i64 :: proc(qb: ^_Query_Builder, key: string, v: i64) {
	_qb_raw(qb, key, fmt.tprintf("%d", v))
}

@(private)
_qb_add_f64 :: proc(qb: ^_Query_Builder, key: string, v: f64) {
	_qb_raw(qb, key, fmt.tprintf("%v", v))
}

@(private)
_qb_add_bool :: proc(qb: ^_Query_Builder, key: string, v: bool) {
	_qb_raw(qb, key, v ? "true" : "false")
}

@(private)
_qb_maybe_string :: proc(qb: ^_Query_Builder, key: string, v: Maybe(string)) {
	if s, ok := v.?; ok {
		_qb_add_string(qb, key, s)
	}
}

@(private)
_qb_maybe_i64 :: proc(qb: ^_Query_Builder, key: string, v: Maybe(i64)) {
	if s, ok := v.?; ok {
		_qb_add_i64(qb, key, s)
	}
}

@(private)
_qb_maybe_f64 :: proc(qb: ^_Query_Builder, key: string, v: Maybe(f64)) {
	if s, ok := v.?; ok {
		_qb_add_f64(qb, key, s)
	}
}

@(private)
_qb_maybe_bool :: proc(qb: ^_Query_Builder, key: string, v: Maybe(bool)) {
	if s, ok := v.?; ok {
		_qb_add_bool(qb, key, s)
	}
}

@(private)
_qb_string :: proc(qb: ^_Query_Builder) -> string {
	return strings.to_string(qb.sb)
}

@(private)
_unreserved :: proc(b: byte) -> bool {
	switch b {
	case 'A'..='Z', 'a'..='z', '0'..='9', '-', '_', '.', '~':
		return true
	}
	return false
}

@(private)
_percent_encode :: proc(s: string) -> string {
	needs := false
	for i in 0..<len(s) {
		if !_unreserved(s[i]) {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	hex := "0123456789ABCDEF"
	sb := strings.builder_make(context.temp_allocator)
	for i in 0..<len(s) {
		b := s[i]
		if _unreserved(b) {
			strings.write_byte(&sb, b)
		} else {
			strings.write_byte(&sb, '%')
			strings.write_byte(&sb, hex[b >> 4])
			strings.write_byte(&sb, hex[b & 0xF])
		}
	}
	return strings.to_string(sb)
}
