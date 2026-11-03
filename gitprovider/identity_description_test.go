package gitprovider_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/fakeprovider"
)

type hiddenIdentityDescriber struct{ *fakeprovider.Full }

func (p hiddenIdentityDescriber) Describe() gp.Info {
	i := p.Full.Describe()
	i.Capabilities.IdentityDescription = false
	return i
}

func TestDescribeIdentityWireGuard(t *testing.T) {
	for _, p := range []gp.Provider{fakeprovider.NewMinimal(), hiddenIdentityDescriber{fakeprovider.New()}} {
		status, raw := gp.Dispatch(context.Background(), p, "describe_identity", []byte(`{}`))
		var env gp.Envelope
		if err := json.Unmarshal(raw, &env); err != nil || status != 501 || env.Error == nil || !errors.Is(env.Error, gp.ErrUnsupported) {
			t.Errorf("unsupported: status=%d body=%s err=%v", status, raw, err)
		}
	}
}
