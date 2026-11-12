package ocrcheck

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type directClient struct {
	*FakeClient
	call func(context.Context, Request) (Response, error)
}

func (c *directClient) Direct(ctx context.Context, request Request) (Response, error) {
	c.DirectCalls++
	return c.call(ctx, request)
}

func TestCheck_LostFinalUsageRetainsReservation(t *testing.T) {
	for _, kind := range []string{"connection", "timeout", "cut_stream"} {
		t.Run(kind, func(t *testing.T) {
			r, s, f, docs := fixture(t)
			second := docs[0]
			second.Entry.ID = 2
			docs = append(docs, second)
			sdk := sdkForTest(func(*http.Request) (*http.Response, error) {
				if kind == "connection" {
					return nil, errors.New("connection lost after send")
				}
				if kind == "timeout" {
					return nil, context.DeadlineExceeded
				}
				partial := strings.Split(streamMessage(true), "event: message_delta")[0]
				return sdkResponse(200, "text/event-stream", partial), nil
			})
			r.Client = &directClient{FakeClient: f, call: sdk.Direct}
			summary, err := r.Run(context.Background(), docs, options())
			if err == nil {
				t.Fatal("unknown billing did not stop spending")
			}
			if f.DirectCalls != 1 || len(s.attempts) != 1 || s.attempts[0].State != "uncertain" || len(s.checks) != 0 {
				t.Fatalf("calls=%d attempts=%+v", f.DirectCalls, s.attempts)
			}
			spent, err := s.Spent(context.Background(), options().CheckerID())
			if err != nil || spent != 320200 || summary.CostMicroUSD != 320200 {
				t.Fatalf("reservation lost: spent=%d summary=%+v err=%v", spent, summary, err)
			}
			if err = run(t, r, docs, options()); err == nil || f.DirectCalls != 1 {
				t.Fatal("another invocation spent while billing was unknown")
			}
		})
	}
}

func TestCheck_ReconcileRequiresPrefixAndNeverAbandons(t *testing.T) {
	for _, kind := range []string{"empty", "wrong_prefix"} {
		t.Run(kind, func(t *testing.T) {
			r, s, c, d := fixture(t)
			o := options()
			o.Batch = true
			c.CreateError = errors.New("lost response")
			_ = run(t, r, d, o)
			c.CreateError = nil
			r.Now = func() time.Time { return s.attempts[0].CreatedAt.Add(48 * time.Hour) }
			if kind == "wrong_prefix" {
				c.Batches = []Batch{{ID: "other-workspace", Ended: true, CreatedAt: s.attempts[0].CreatedAt}}
				c.Results = []BatchResult{{CustomID: strings.Repeat("f", 32) + "-1", Type: "succeeded", Response: c.Response}}
			}
			o.Reconcile = true
			if err := run(t, r, d, o); err != nil {
				t.Fatal(err)
			}
			if s.attempts[0].State != "uncertain" {
				t.Fatalf("unmatched listing released paid intent: %+v", s.attempts)
			}
		})
	}
}

func TestCheck_OperatorAbandonKeepsReservation(t *testing.T) {
	for _, age := range []time.Duration{time.Minute, 23 * time.Hour, 24 * time.Hour} {
		t.Run(age.String(), func(t *testing.T) {
			r, s, c, d := fixture(t)
			o := options()
			o.Batch = true
			c.CreateError = errors.New("lost batch response")
			_ = run(t, r, d, o)
			submission := s.attempts[0].SubmissionID
			r.Now = func() time.Time { return s.attempts[0].CreatedAt.Add(age) }
			o = options()
			o.Abandon = submission
			err := run(t, r, nil, o)
			if age < 24*time.Hour {
				if err == nil || s.attempts[0].State != "uncertain" {
					t.Fatal("abandoned before 24 hours")
				}
				return
			}
			if err != nil || s.attempts[0].State != "abandoned" {
				t.Fatalf("operator could not abandon: %v %+v", err, s.attempts)
			}
			spent, err := s.Spent(context.Background(), o.CheckerID())
			if err != nil || spent != 160100 {
				t.Fatalf("abandonment released reservation: %d %v", spent, err)
			}
			o.Abandon = ""
			o.Batch = true
			o.MaxUSD = .2
			c.CreateError = nil
			if err = run(t, r, d, o); !errors.Is(err, ErrBudget) || c.BatchCalls != 1 {
				t.Fatalf("abandoned work was not charged: %v calls=%d", err, c.BatchCalls)
			}
		})
	}
}

func TestCheck_IntentAndReservationExistBeforeProviderCall(t *testing.T) {
	r, s, c, d := fixture(t)
	r.Client = &directClient{FakeClient: c, call: func(context.Context, Request) (Response, error) {
		open, err := s.Open(context.Background(), options().CheckerID())
		if err != nil || len(open) != 1 || open[0].State != "preparing" {
			t.Fatal("provider called before durable intent")
		}
		spent, err := s.Spent(context.Background(), options().CheckerID())
		if err != nil || spent != 320200 {
			t.Fatalf("provider called before reservation: %d %v", spent, err)
		}
		return c.Response, nil
	}}
	if err := run(t, r, d, options()); err != nil {
		t.Fatal(err)
	}
}

func TestCheck_DirectFailureRequiresFinalUsageOrProvenRejection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		requestID bool
		body      string
		want      string
		cost      int64
	}{
		{name: "final usage before cut", body: streamMessage(false), want: "failed", cost: 700},
		{name: "definite rejection", status: 400, requestID: true, want: "failed"},
		{name: "rate limit", status: 429, requestID: true, want: "failed"},
		{name: "missing request identity", status: 400, want: "uncertain", cost: 320200},
		{name: "provider timeout", status: 408, requestID: true, want: "uncertain", cost: 320200},
		{name: "provider failure", status: 529, requestID: true, want: "uncertain", cost: 320200},
		{name: "stop without final usage", body: strings.Split(streamMessage(true), "event: message_delta")[0] + "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", want: "uncertain", cost: 320200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, s, c, d := fixture(t)
			sdk := sdkForTest(func(*http.Request) (*http.Response, error) {
				if tc.status == 0 {
					return sdkResponse(200, "text/event-stream", tc.body), nil
				}
				response := sdkResponse(tc.status, "application/json", `{"type":"error","error":{"type":"invalid_request_error","message":"unavailable"}}`)
				if !tc.requestID {
					response.Header.Del("Request-Id")
				}
				return response, nil
			})
			r.Client = &directClient{FakeClient: c, call: sdk.Direct}
			err := run(t, r, d, options())
			if (err != nil) != (tc.want == "uncertain") || s.attempts[0].State != tc.want {
				t.Fatalf("state=%s error=%v", s.attempts[0].State, err)
			}
			spent, err := s.Spent(context.Background(), options().CheckerID())
			if err != nil || spent != tc.cost {
				t.Fatalf("charge=%d want=%d err=%v", spent, tc.cost, err)
			}
		})
	}
}

func TestCheck_DirectAbandonImmediatelyKeepsReservationAndAllowsRetry(t *testing.T) {
	r, repo, client, docs := fixture(t)
	r.Client = &directClient{FakeClient: client, call: func(context.Context, Request) (Response, error) { return Response{}, errors.New("lost response") }}
	o := options()
	if err := run(t, r, docs, o); err == nil {
		t.Fatal("expected uncertain attempt")
	}
	attempt := repo.attempts[0]
	r.Now = func() time.Time { return attempt.CreatedAt.Add(time.Minute) }
	o.Abandon = attempt.SubmissionID
	if err := run(t, r, nil, o); err != nil {
		t.Fatal(err)
	}
	spent, err := repo.Spent(context.Background(), o.CheckerID())
	if err != nil || spent != 320200 || repo.attempts[0].State != "abandoned" {
		t.Fatalf("spent=%d attempts=%+v err=%v", spent, repo.attempts, err)
	}
	o.Abandon = ""
	r.Client = client
	if err := run(t, r, docs, o); err != nil || client.DirectCalls != 2 || len(repo.attempts) != 2 {
		t.Fatalf("calls=%d attempts=%+v err=%v", client.DirectCalls, repo.attempts, err)
	}
}
