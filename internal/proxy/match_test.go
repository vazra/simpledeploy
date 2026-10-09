package proxy

import (
	"reflect"
	"testing"
)

func TestOrderRoutesPerDomain(t *testing.T) {
	in := []Route{
		{Domain: "a.com", Upstream: "catchall", Protocol: "http"},
		{Domain: "b.com", Upstream: "b", Protocol: "http"},
		{Domain: "a.com", Upstream: "ws", Protocol: "http", Path: "/ws*"},
		{Domain: "a.com", Upstream: "grpc", Protocol: "grpc"},
		{Domain: "a.com", Upstream: "wsdeep", Protocol: "http", Path: "/ws/deep*"},
		{Domain: "a.com", Upstream: "h2c", Protocol: "h2c", Path: "/h2*"},
	}
	got := orderRoutes(in)
	var ups []string
	for _, r := range got {
		ups = append(ups, r.Upstream)
	}
	want := []string{"grpc", "wsdeep", "h2c", "ws", "catchall", "b"} // equal-length paths tie-break lexically
	if !reflect.DeepEqual(ups, want) {
		t.Fatalf("order = %v, want %v", ups, want)
	}
	if in[0].Upstream != "catchall" || in[3].Upstream != "grpc" {
		t.Error("orderRoutes mutated its input")
	}
}

func TestRouteMatcherHTTPCatchAll(t *testing.T) {
	m := routeMatcher(Route{Domain: "a.com", Protocol: "http"})
	want := map[string]interface{}{"host": []string{"a.com"}}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("matcher = %#v, want %#v", m, want)
	}
}

func TestRouteMatcherPath(t *testing.T) {
	m := routeMatcher(Route{Domain: "a.com", Path: "/ws*"})
	if !reflect.DeepEqual(m["path"], []string{"/ws*"}) {
		t.Fatalf("path = %#v, want [/ws*]", m["path"])
	}
	if _, ok := m["header"]; ok {
		t.Error("path route must not have header matcher")
	}
}

func TestRouteMatcherGRPC(t *testing.T) {
	m := routeMatcher(Route{Domain: "a.com", Protocol: "grpc"})
	wantHeader := map[string][]string{"Content-Type": {"application/grpc*"}}
	if !reflect.DeepEqual(m["header"], wantHeader) {
		t.Fatalf("header = %#v, want %#v", m["header"], wantHeader)
	}
	wantNot := []interface{}{
		map[string]interface{}{"header": map[string][]string{"Content-Type": {"application/grpc-web*"}}},
	}
	if !reflect.DeepEqual(m["not"], wantNot) {
		t.Fatalf("not = %#v, want %#v", m["not"], wantNot)
	}
}

func TestReverseProxyHandlerTransport(t *testing.T) {
	for _, proto := range []string{"h2c", "grpc"} {
		h := reverseProxyHandler(Route{Upstream: "co:50051", Protocol: proto})
		wantTransport := map[string]interface{}{"protocol": "http", "versions": []string{"h2c"}}
		if !reflect.DeepEqual(h["transport"], wantTransport) {
			t.Errorf("%s transport = %#v, want %#v", proto, h["transport"], wantTransport)
		}
		if h["flush_interval"] != -1 {
			t.Errorf("%s flush_interval = %#v, want -1", proto, h["flush_interval"])
		}
	}
	h := reverseProxyHandler(Route{Upstream: "co:8001", Protocol: "http"})
	if _, ok := h["transport"]; ok {
		t.Error("http route must use default transport")
	}
	if _, ok := h["flush_interval"]; ok {
		t.Error("http route must not set flush_interval")
	}
	if h["handler"] != "reverse_proxy" {
		t.Errorf("handler = %v", h["handler"])
	}
	ups := h["upstreams"].([]interface{})
	if ups[0].(map[string]interface{})["dial"] != "co:8001" {
		t.Errorf("dial = %v", ups[0])
	}
}
