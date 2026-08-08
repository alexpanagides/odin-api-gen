// End-to-end tests for the generated SDK using a mock transport: they verify
// URL/query construction, auth headers, request-body marshaling (omitempty),
// response decoding (including Maybe/null), and error mapping — all against
// the real core:encoding/json, with no network.
package sdk_tests

import "base:runtime"
import "core:strings"
import "core:testing"
import ps "../sdk/pocketsmith"

Capture :: struct {
	// what the mock returns
	status:   int,
	response: string,
	// what the SDK sent (cloned to context.temp_allocator)
	method:  string,
	url:     string,
	body:    string,
	headers: string, // "Name: value\n" per header
}

mock_transport :: proc(data: rawptr, req: ps.Http_Request, allocator: runtime.Allocator) -> (resp: ps.Http_Response, terr: Maybe(ps.Transport_Error)) {
	cap := (^Capture)(data)
	cap.method = strings.clone(req.method, context.temp_allocator)
	cap.url = strings.clone(req.url, context.temp_allocator)
	cap.body = strings.clone(string(req.body), context.temp_allocator)
	sb := strings.builder_make(context.temp_allocator)
	for h in req.headers {
		strings.write_string(&sb, h[0])
		strings.write_string(&sb, ": ")
		strings.write_string(&sb, h[1])
		strings.write_byte(&sb, '\n')
	}
	cap.headers = strings.to_string(sb)

	body := strings.clone(cap.response, allocator)
	resp = ps.Http_Response{status = cap.status, body = transmute([]u8)body}
	return
}

mock_client :: proc(cap: ^Capture) -> ps.Client {
	c := ps.client_from_developer_key("test-key")
	c.transport = mock_transport
	c.transport_data = cap
	return c
}

@(test)
test_get_user_decodes_and_authenticates :: proc(t: ^testing.T) {
	cap := Capture{
		status   = 200,
		response = `{"id": 42, "login": "alexis", "email": "a@b.c", "some_future_field": [1, 2, 3]}`,
	}
	c := mock_client(&cap)
	user, err := ps.get_user(&c, 42, context.temp_allocator)
	testing.expect(t, err == nil, "unexpected error")
	testing.expect_value(t, user.id, 42)
	testing.expect_value(t, user.login, "alexis")
	testing.expect_value(t, cap.method, "GET")
	testing.expect_value(t, cap.url, "https://api.pocketsmith.com/v2/users/42")
	testing.expect(t, strings.contains(cap.headers, "X-Developer-Key: test-key\n"), "developer key header missing")
	testing.expect(t, cap.body == "", "GET must not send a body")
}

@(test)
test_oauth_bearer_header :: proc(t: ^testing.T) {
	cap := Capture{status = 200, response = `{"id": 1}`}
	c := ps.client_from_oauth_token("tok-123")
	c.transport = mock_transport
	c.transport_data = &cap
	_, err := ps.get_authorised_user(&c, context.temp_allocator)
	testing.expect(t, err == nil, "unexpected error")
	testing.expect_value(t, cap.url, "https://api.pocketsmith.com/v2/me")
	testing.expect(t, strings.contains(cap.headers, "Authorization: Bearer tok-123\n"), "bearer header missing")
}

@(test)
test_query_building_and_percent_encoding :: proc(t: ^testing.T) {
	cap := Capture{
		status   = 200,
		response = `[{"id": 1, "payee": "St Martins", "amount": 34.6, "labels": ["foo", "bar"]}]`,
	}
	c := mock_client(&cap)
	txns, err := ps.list_transactions_in_user(&c, 7, ps.List_Transactions_In_User_Options{
		start_date = "2024-01-01",
		search     = "cheese & wine",
		page       = 2,
	}, context.temp_allocator)
	testing.expect(t, err == nil, "unexpected error")
	testing.expect_value(t, len(txns), 1)
	testing.expect_value(t, txns[0].amount, 34.6)
	testing.expect_value(t, len(txns[0].labels), 2)
	testing.expect_value(t, cap.url, "https://api.pocketsmith.com/v2/users/7/transactions?start_date=2024-01-01&search=cheese%20%26%20wine&page=2")
}

@(test)
test_update_body_omits_unset_fields :: proc(t: ^testing.T) {
	cap := Capture{status = 200, response = `{"id": 5, "title": "Beer", "refund_behaviour": null}`}
	c := mock_client(&cap)
	cat, err := ps.update_category(&c, 5, ps.Update_Category_Body{title = "Beer"}, context.temp_allocator)
	testing.expect(t, err == nil, "unexpected error")
	testing.expect_value(t, cap.method, "PUT")
	testing.expect_value(t, cap.body, `{"title":"Beer"}`)
	testing.expect(t, strings.contains(cap.headers, "Content-Type: application/json\n"), "content type missing")
	testing.expect_value(t, cat.title, "Beer")
	testing.expect(t, cat.refund_behaviour == nil, "null must decode to nil Maybe")
}

@(test)
test_api_error_mapping :: proc(t: ^testing.T) {
	cap := Capture{status = 404, response = `{"error": "Not Found"}`}
	c := mock_client(&cap)
	_, err := ps.get_user(&c, 1, context.temp_allocator)
	api, ok := err.(ps.Api_Error)
	testing.expect(t, ok, "expected Api_Error")
	testing.expect_value(t, api.status, 404)
	testing.expect_value(t, api.message, "Not Found")
}

@(test)
test_delete_no_content :: proc(t: ^testing.T) {
	cap := Capture{status = 204, response = ""}
	c := mock_client(&cap)
	err := ps.delete_transaction(&c, 9, context.temp_allocator)
	testing.expect(t, err == nil, "unexpected error")
	testing.expect_value(t, cap.method, "DELETE")
	testing.expect_value(t, cap.url, "https://api.pocketsmith.com/v2/transactions/9")
}

@(test)
test_required_query_params :: proc(t: ^testing.T) {
	cap := Capture{status = 200, response = `[]`}
	c := mock_client(&cap)
	_, err := ps.get_budget_summary_for_user(&c, 3, "months", 1, "2024-01-01", "2024-06-30", context.temp_allocator)
	testing.expect(t, err == nil, "unexpected error")
	testing.expect_value(t, cap.url, "https://api.pocketsmith.com/v2/users/3/budget_summary?period=months&interval=1&start_date=2024-01-01&end_date=2024-06-30")
}
