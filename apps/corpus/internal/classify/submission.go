package classify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"github.com/anthropics/anthropic-sdk-go/v2"
)

type Item struct {
	Key         string           `json:"key"`
	RequestHash string           `json:"request_hash"`
	InputTokens int64            `json:"counted_input_tokens"`
	Article     *ArticleRequest  `json:"article_request,omitempty"`
	ArticleMeta *ArticleMetadata `json:"article_metadata,omitempty"`
}

type Submission struct {
	ID              string             `json:"id"`
	RunID           int64              `json:"run_id"`
	Country         string             `json:"country"`
	Release         string             `json:"release"`
	ScreenHash      string             `json:"screen_hash"`
	Recipe          Recipe             `json:"recipe"`
	Families        []Family           `json:"families"`
	Items           []Item             `json:"items"`
	ItemsHash       string             `json:"items_hash"`
	State           string             `json:"state"`
	BatchID         string             `json:"provider_job_id"`
	CreatedAt       time.Time          `json:"created_at"`
	SubmittedAt     *time.Time         `json:"submitted_at"`
	DeadlineAt      *time.Time         `json:"deadline_at"`
	Budget          string             `json:"budget"`
	BudgetMicroUSD  int64              `json:"budget_micro_usd"`
	ReserveMicroUSD int64              `json:"reserved_micro_usd"`
	Settled         bool               `json:"settled"`
	CostMicroUSD    int64              `json:"cost_micro_usd"`
	Results         map[string]Outcome `json:"results"`
}

type Repository interface {
	Lock(context.Context) (func(), error)
	List(context.Context) ([]Submission, error)
	Save(context.Context, Submission) error
	Restore(context.Context, []Submission) error
}

type Client interface {
	CountTokens(context.Context, anthropic.MessageNewParams) (int64, error)
	Create(context.Context, []batchapi.Request) (anthropic.MessageBatch, error)
	Get(context.Context, string) (anthropic.MessageBatch, error)
	Results(context.Context, string) ([]batchapi.Result, error)
}

type Runner struct {
	Repository Repository
	Client     Client
	IntentDir  string
	Now        func() time.Time
	Wait       func(context.Context, time.Duration) error
}

type Options struct {
	RunID            int64
	Country, Release string
	Documents        int
	Seed             int64
	Recipe           Recipe
	Budget           string
	BudgetMicroUSD   int64
	Out              string
	DryRun           bool
	article          *articleWork
}

type Summary struct {
	Requests        int    `json:"requests"`
	InputTokens     int64  `json:"counted_input_tokens"`
	ReserveMicroUSD int64  `json:"reserve_micro_usd"`
	LedgerMicroUSD  int64  `json:"ledger_micro_usd"`
	Admitted        bool   `json:"admitted"`
	SubmissionID    string `json:"submission_id,omitempty"`
}

func (o Options) validate() error {
	if o.RunID < 1 || o.Country != "vn" || o.Release == "" || o.Documents < 1 || o.Documents > 400 || o.Out == "" {
		return fmt.Errorf("invalid screening options: require release, output and 1 to 400 documents")
	}
	amounts := map[string]int64{"pilot": 15000000, "full": 100000000, "cap": 150000000}
	if amounts[o.Budget] == 0 || amounts[o.Budget] != o.BudgetMicroUSD {
		return fmt.Errorf("budget name and amount must match pilot 15 USD, full 100 USD or cap 150 USD")
	}
	return o.Recipe.Validate()
}

func customID(id string, index int) string { return fmt.Sprintf("%s-%06d", id, index) }

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("create submission identity: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func itemsHash(items []Item) string { b, _ := json.Marshal(items); return digest(b) }

func ledgerCost(rows []Submission) (int64, error) {
	var total int64
	for _, s := range rows {
		charge := s.ReserveMicroUSD
		if s.Settled || s.CostMicroUSD > charge {
			charge = s.CostMicroUSD
		}
		if charge < 0 || charge > math.MaxInt64-total {
			return 0, fmt.Errorf("invalid submission ledger cost")
		}
		total += charge
	}
	return total, nil
}

func nonterminal(state string) bool {
	return state == "preparing" || state == "submitted" || state == "uncertain" || state == "cancelling"
}
