//go:build mutants

package fakeprovider_test

// Mutation checks for the conformance suite: each provider below has ONE
// deliberate defect, and the suite is expected to FAIL on it. Run with
//
//	go test -tags mutants ./gitprovider/fakeprovider/ -run TestMutant
//
// and confirm every TestMutant_* reports FAIL (scripts/check-mutants.sh does).

import (
	"context"
	"errors"
	"testing"
	"time"

	gp "github.com/Paca-AI/plugin-sdk-go/gitprovider"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/conformance"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/fakeprovider"
)

func run(t *testing.T, mk func() gp.Provider) {
	h := fakeprovider.Harness(false)
	h.New = func(*testing.T) gp.Provider { return mk() }
	conformance.Run(t, h)
}

type noVerify struct{ *fakeprovider.Full }

func (n noVerify) VerifyAndParseWebhook(ctx context.Context, r gp.WebhookRequest) ([]gp.Event, error) {
	r.Headers = map[string]string{"x-fake-signature": fakeprovider.Sign(r.Body)} // accepts anything
	return n.Full.VerifyAndParseWebhook(ctx, r)
}
func TestMutant_NoSignatureVerification(t *testing.T) {
	run(t, func() gp.Provider { return noVerify{fakeprovider.New()} })
}

type leaky struct{ *fakeprovider.Full }

func (l leaky) GetChangeRequest(ctx context.Context, r gp.CRRequest) (gp.ChangeRequest, error) {
	cr, err := l.Full.GetChangeRequest(ctx, r)
	if err != nil {
		return cr, gp.Errorf(gp.CodeNotFound, "not found (token %s)", fakeprovider.StaticSecret)
	}
	return cr, nil
}
func TestMutant_ErrorLeaksSecret(t *testing.T) {
	run(t, func() gp.Provider { return leaky{fakeprovider.New()} })
}

type unclassified struct{ *fakeprovider.Full }

func (u unclassified) GetIssue(ctx context.Context, r gp.IssueRequest) (gp.Issue, error) {
	if is, err := u.Full.GetIssue(ctx, r); err != nil {
		return is, errors.New("HTTP 404 from upstream")
	} else {
		return is, nil
	}
}
func TestMutant_UnclassifiedError(t *testing.T) {
	run(t, func() gp.Provider { return unclassified{fakeprovider.New()} })
}

type dishonest struct{ *fakeprovider.Full }

func (d dishonest) Describe() gp.Info {
	i := d.Full.Describe()
	i.Capabilities.Merge = false // implements Merger but hides it
	return i
}
func TestMutant_DishonestCapability(t *testing.T) {
	run(t, func() gp.Provider { return dishonest{fakeprovider.New()} })
}

type noExpiry struct{ *fakeprovider.Full }

func (n noExpiry) MintCredential(ctx context.Context, r gp.MintRequest) (gp.Credential, error) {
	c, err := n.Full.MintCredential(ctx, r)
	c.ExpiresAt = time.Time{}
	return c, err
}
func TestMutant_EphemeralWithoutExpiry(t *testing.T) {
	run(t, func() gp.Provider { return noExpiry{fakeprovider.New()} })
}

type silentTrunc struct{ *fakeprovider.Full }

func (s silentTrunc) GetDiff(ctx context.Context, r gp.DiffRequest) (gp.Diff, error) {
	max := r.MaxBytes
	r.MaxBytes = 0 // fetch it all, then cut without saying so
	d, err := s.Full.GetDiff(ctx, r)
	if err == nil && max > 0 && len(d.Text) > max {
		d.Text, d.Truncated = d.Text[:max], false
	}
	return d, err
}
func TestMutant_SilentTruncation(t *testing.T) {
	run(t, func() gp.Provider { return silentTrunc{fakeprovider.New()} })
}

type unstableIDs struct {
	*fakeprovider.Full
	n int
}

func (u *unstableIDs) VerifyAndParseWebhook(ctx context.Context, r gp.WebhookRequest) ([]gp.Event, error) {
	evs, err := u.Full.VerifyAndParseWebhook(ctx, r)
	for i := range evs {
		u.n++
		evs[i].ID += string(rune('a' + u.n%26))
	}
	return evs, err
}
func TestMutant_NonDeterministicEventIDs(t *testing.T) {
	run(t, func() gp.Provider { return &unstableIDs{Full: fakeprovider.New()} })
}

type secretInEvent struct{ *fakeprovider.Full }

func (s secretInEvent) VerifyAndParseWebhook(ctx context.Context, r gp.WebhookRequest) ([]gp.Event, error) {
	evs, err := s.Full.VerifyAndParseWebhook(ctx, r)
	for i := range evs {
		evs[i].RawRef = fakeprovider.WebhookSecret
	}
	return evs, err
}
func TestMutant_SecretInEvent(t *testing.T) {
	run(t, func() gp.Provider { return secretInEvent{fakeprovider.New()} })
}

type mergeNoGuard struct{ *fakeprovider.Full }

func (m mergeNoGuard) MergeChangeRequest(ctx context.Context, r gp.MergeRequest) (gp.MergeResult, error) {
	cr, _ := m.Full.GetChangeRequest(ctx, gp.CRRequest{Target: r.Target, Number: r.Number})
	r.ExpectedHeadSHA = cr.Head.SHA // ignores the caller's expected head
	return m.Full.MergeChangeRequest(ctx, r)
}
func TestMutant_MergeIgnoresExpectedHead(t *testing.T) {
	run(t, func() gp.Provider { return mergeNoGuard{fakeprovider.New()} })
}

// A grant callback that works for any state, or more than once, lets a
// replayed or forged redirect grant access.
type replayableGrant struct{ *fakeprovider.Full }

func (r replayableGrant) CompleteAccessGrant(ctx context.Context, req gp.CompleteAccessRequest) (gp.AccessGrant, error) {
	return gp.AccessGrant{Owner: req.Params["owner"]}, nil
}
func TestMutant_AccessGrantAcceptsAnyState(t *testing.T) {
	run(t, func() gp.Provider { return replayableGrant{fakeprovider.New()} })
}

type reusableGrant struct {
	*fakeprovider.Full
	step gp.SetupStep
}

func (r *reusableGrant) BeginAccessGrant(ctx context.Context, req gp.BeginAccessRequest) (gp.SetupStep, error) {
	s, err := r.Full.BeginAccessGrant(ctx, req)
	if err == nil {
		r.step = s
	}
	return s, err
}
func (r *reusableGrant) CompleteAccessGrant(ctx context.Context, req gp.CompleteAccessRequest) (gp.AccessGrant, error) {
	g, err := r.Full.CompleteAccessGrant(ctx, req)
	if err != nil && req.State == r.step.State && r.step.State != "" {
		return gp.AccessGrant{Owner: req.Params["owner"]}, nil // forgives a replay
	}
	return g, err
}
func TestMutant_AccessGrantStateIsReusable(t *testing.T) {
	run(t, func() gp.Provider { return &reusableGrant{Full: fakeprovider.New()} })
}

// A branch create that forgives an existing name hides lost races.
type branchOverwrite struct{ *fakeprovider.Full }

func (b branchOverwrite) CreateBranch(ctx context.Context, r gp.CreateBranchRequest) (gp.BranchInfo, error) {
	info, err := b.Full.CreateBranch(ctx, r)
	var pe *gp.Error
	if errors.As(err, &pe) && pe.Code == gp.CodeConflict {
		return b.Full.GetBranch(ctx, gp.GetBranchRequest{Target: r.Target, Name: r.Name})
	}
	return info, err
}
func TestMutant_BranchCreateForgivesExisting(t *testing.T) {
	run(t, func() gp.Provider { return branchOverwrite{fakeprovider.New()} })
}

// A branch create that ignores From always branches off the default.
type branchIgnoresFrom struct{ *fakeprovider.Full }

func (b branchIgnoresFrom) CreateBranch(ctx context.Context, r gp.CreateBranchRequest) (gp.BranchInfo, error) {
	r.From = ""
	return b.Full.CreateBranch(ctx, r)
}
func TestMutant_BranchCreateIgnoresFrom(t *testing.T) {
	run(t, func() gp.Provider { return branchIgnoresFrom{fakeprovider.New()} })
}
