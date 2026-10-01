// Default HTTP transport backed by Odin's vendor:curl bindings (handles TLS).
// Written by hand, copied verbatim by odin-api-gen.
//
// Any other HTTP client can be used instead by setting Client.transport;
// nothing else in the SDK depends on curl.
package PACKAGE_NAME

import "base:runtime"
import "core:c"
import "core:fmt"
import "core:strings"
import curl "vendor:curl"

@(private) _curl_global_ready: bool

@(private)
_Curl_Buf :: struct {
    data: [dynamic]u8,
    ctx: runtime.Context,
}

@(private)
_curl_write_cb :: proc "c" (buffer: [^]byte, size: c.size_t, nitems: c.size_t, outstream: rawptr) -> c.size_t {
    buf := (^_Curl_Buf)(outstream)
    context = buf.ctx
    n := int(size) * int(nitems)
    if n > 0 {
        append(&buf.data, ..buffer[:n])
    }
    return size * nitems
}

_default_transport :: proc(data: rawptr, req: Http_Request, allocator: runtime.Allocator) -> (resp: Http_Response, terr: Maybe(Transport_Error)) {
    if !_curl_global_ready {
        curl.global_init(curl.GLOBAL_DEFAULT)
        _curl_global_ready = true
    }

    h := curl.easy_init()
    if h == nil {
        terr = Transport_Error{ message = "curl_easy_init failed" }
        return
    }
    defer curl.easy_cleanup(h)

    curl.easy_setopt(h, .URL, strings.clone_to_cstring(req.url, context.temp_allocator))
    curl.easy_setopt(h, .CUSTOMREQUEST, strings.clone_to_cstring(req.method, context.temp_allocator))
    curl.easy_setopt(h, .FOLLOWLOCATION, c.long(1))

    headers: ^curl.slist
    for hdr in req.headers {
        headers = curl.slist_append(headers, fmt.ctprintf("%s: %s", hdr[0], hdr[1]))
    }
    defer if headers != nil {
        curl.slist_free_all(headers)
    }
    if headers != nil {
        curl.easy_setopt(h, .HTTPHEADER, headers)
    }

    if req.body != nil {
        curl.easy_setopt(h, .POSTFIELDSIZE, c.long(len(req.body)))
        curl.easy_setopt(h, .POSTFIELDS, raw_data(req.body))
    }

    buf := _Curl_Buf{ ctx = context }
    buf.data.allocator = allocator
    cb : curl.write_callback = _curl_write_cb
    curl.easy_setopt(h, .WRITEFUNCTION, cb)
    curl.easy_setopt(h, .WRITEDATA, rawptr(&buf))

    code := curl.easy_perform(h)
    if code != .E_OK {
        delete(buf.data)
        terr = Transport_Error{ code = int(code), message = string(curl.easy_strerror(code)) }
        return
    }

    status: c.long
    curl.easy_getinfo(h, .RESPONSE_CODE, &status)
    resp = Http_Response{ status = int(status), body = buf.data[:] }
    return
}
