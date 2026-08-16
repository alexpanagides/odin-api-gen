// Default HTTP transport backed by the system libcurl (handles TLS).
// Written by hand, copied verbatim by odin-api-gen.
//
// Any other HTTP client can be used instead by setting Client.transport;
// nothing else in the SDK depends on libcurl.
package PACKAGE_NAME

import "base:runtime"
import "core:c"
import "core:fmt"
import "core:strings"

when ODIN_OS == .Windows {

// No system libcurl is assumed on Windows; provide your own transport.
    _default_transport :: proc(data: rawptr, req: Http_Request, allocator: runtime.Allocator) -> (resp: Http_Response, terr: Maybe(Transport_Error)) {
        terr = Transport_Error{ message = "no default transport on Windows: set Client.transport" }
        return
    }

} else {

    foreign import libcurl "system:curl"

    @(private) _CURL_Handle :: distinct rawptr
    @(private) _CURL_Code :: distinct c.int
    @(private) _CURL_Option :: distinct c.int
    @(private) _CURL_Info :: distinct c.int
    @(private) _CURL_Slist :: struct {
    }

    @(private) _CURLOPT_WRITEDATA :: _CURL_Option(10001)
    @(private) _CURLOPT_URL :: _CURL_Option(10002)
    @(private) _CURLOPT_POSTFIELDS :: _CURL_Option(10015)
    @(private) _CURLOPT_HTTPHEADER :: _CURL_Option(10023)
    @(private) _CURLOPT_CUSTOMREQUEST :: _CURL_Option(10036)
    @(private) _CURLOPT_FOLLOWLOCATION :: _CURL_Option(52)
    @(private) _CURLOPT_POSTFIELDSIZE :: _CURL_Option(60)
    @(private) _CURLOPT_WRITEFUNCTION :: _CURL_Option(20011)
    @(private) _CURLINFO_RESPONSE_CODE :: _CURL_Info(0x200002)
    @(private) _CURL_GLOBAL_DEFAULT :: c.long(3)

    @(private)
    _Curl_Write_Cb :: #type proc "c" (ptr: rawptr, size: c.size_t, nmemb: c.size_t, userdata: rawptr) -> c.size_t

    @(private, default_calling_convention="c")
    foreign libcurl {
        curl_global_init :: proc(flags: c.long) -> _CURL_Code ---
        curl_easy_init :: proc() -> _CURL_Handle ---
        curl_easy_cleanup :: proc(h: _CURL_Handle) ---
        curl_easy_perform :: proc(h: _CURL_Handle) -> _CURL_Code ---
        curl_easy_strerror :: proc(code: _CURL_Code) -> cstring ---
        curl_slist_append :: proc(l: ^_CURL_Slist, s: cstring) -> ^_CURL_Slist ---
        curl_slist_free_all :: proc(l: ^_CURL_Slist) ---

        // curl_easy_setopt/getinfo are C-variadic.
        curl_easy_setopt :: proc(h: _CURL_Handle, opt: _CURL_Option, #c_vararg args: ..any) -> _CURL_Code ---
        curl_easy_getinfo :: proc(h: _CURL_Handle, info: _CURL_Info, #c_vararg args: ..any) -> _CURL_Code ---
    }

    @(private) _curl_global_ready: bool

    @(private)
    _Curl_Buf :: struct {
        data: [dynamic]u8,
        ctx: runtime.Context,
    }

    @(private)
    _curl_write_cb :: proc "c" (ptr: rawptr, size: c.size_t, nmemb: c.size_t, userdata: rawptr) -> c.size_t {
        buf := (^_Curl_Buf)(userdata)
        context = buf.ctx
        n := int(size) * int(nmemb)
        if n > 0 {
            bytes := ([^]u8)(ptr)[:n]
            append(&buf.data, ..bytes)
        }
        return size * nmemb
    }

    _default_transport :: proc(data: rawptr, req: Http_Request, allocator: runtime.Allocator) -> (resp: Http_Response, terr: Maybe(Transport_Error)) {
        if !_curl_global_ready {
            curl_global_init(_CURL_GLOBAL_DEFAULT)
            _curl_global_ready = true
        }

        h := curl_easy_init()
        if h == nil {
            terr = Transport_Error{ message = "curl_easy_init failed" }
            return
        }
        defer curl_easy_cleanup(h)

        curl_easy_setopt(h, _CURLOPT_URL, strings.clone_to_cstring(req.url, context.temp_allocator))
        curl_easy_setopt(h, _CURLOPT_CUSTOMREQUEST, strings.clone_to_cstring(req.method, context.temp_allocator))
        curl_easy_setopt(h, _CURLOPT_FOLLOWLOCATION, c.long(1))

        headers: ^_CURL_Slist
        for hdr in req.headers {
            headers = curl_slist_append(headers, fmt.ctprintf("%s: %s", hdr[0], hdr[1]))
        }
        defer if headers != nil {
            curl_slist_free_all(headers)
        }
        if headers != nil {
            curl_easy_setopt(h, _CURLOPT_HTTPHEADER, headers)
        }

        if req.body != nil {
            curl_easy_setopt(h, _CURLOPT_POSTFIELDSIZE, c.long(len(req.body)))
            curl_easy_setopt(h, _CURLOPT_POSTFIELDS, raw_data(req.body))
        }

        buf := _Curl_Buf{ ctx = context }
        buf.data.allocator = allocator
        cb : _Curl_Write_Cb = _curl_write_cb
        curl_easy_setopt(h, _CURLOPT_WRITEFUNCTION, cb)
        curl_easy_setopt(h, _CURLOPT_WRITEDATA, rawptr(&buf))

        code := curl_easy_perform(h)
        if code != 0 {
            delete(buf.data)
            terr = Transport_Error{ code = int(code), message = string(curl_easy_strerror(code)) }
            return
        }

        status: c.long
        curl_easy_getinfo(h, _CURLINFO_RESPONSE_CODE, &status)
        resp = Http_Response{ status = int(status), body = buf.data[:] }
        return
    }

}
