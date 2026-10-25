package serve_test

import (
	"encoding/json"
	"testing"

	plugin "github.com/Paca-AI/plugin-sdk-go"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/fakeprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/serve"
	"github.com/Paca-AI/plugin-sdk-go/plugintest"
)

type host struct{ p gitprovider.Provider }

func (h host) Init(ctx *plugin.Context) error { serve.Register(ctx, h.p); return nil }
func (host) Shutdown()                        {}

func setup(t *testing.T, p gitprovider.Provider) *plugintest.Context {
	t.Helper()
	tc := plugintest.NewContext(t)
	if err := (host{p}).Init(tc.PluginContext()); err != nil {
		t.Fatal(err)
	}
	return tc
}

func TestRegister_DescribeOverRoute(t *testing.T) {
	tc := setup(t, fakeprovider.New())
	res := tc.Call("POST", "/_git/describe", plugintest.Request{PathParams: map[string]string{"op": "describe"}})
	if res.StatusCode != 200 {
		t.Fatalf("status %d: %s", res.StatusCode, res.BodyString())
	}
	var env gitprovider.Envelope
	if err := json.Unmarshal(res.Body, &env); err != nil || !env.OK {
		t.Fatalf("envelope: %v %s", err, res.BodyString())
	}
	var info gitprovider.Info
	if err := json.Unmarshal(env.Result, &info); err != nil || info.SPIVersion != gitprovider.SPIVersion {
		t.Fatalf("info: %+v err=%v", info, err)
	}
}

func TestRegister_ErrorsCarryHTTPStatus(t *testing.T) {
	tc := setup(t, fakeprovider.New())
	res := tc.Call("POST", "/_git/get_change_request", plugintest.Request{
		PathParams: map[string]string{"op": "get_change_request"},
		Body:       []byte(`{"target":{"repo":{"full_path":"nope/none"}},"number":"1"}`),
	})
	if res.StatusCode != 404 {
		t.Fatalf("want 404, got %d: %s", res.StatusCode, res.BodyString())
	}
}
