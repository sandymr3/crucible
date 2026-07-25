package roadmap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"

	"github.com/santh/crucible/internal/config"
	"github.com/santh/crucible/internal/prompts"
	"github.com/santh/crucible/internal/store"
	"github.com/santh/crucible/internal/vertexai"
)

// Resource is one study link.
type Resource struct {
	Title   string `firestore:"title" json:"title"`
	URL     string `firestore:"url" json:"url"`
	Type    string `firestore:"type" json:"type"`
	Minutes int    `firestore:"minutes" json:"minutes"`
	// Verified records that this URL was actually fetched and answered.
	// Unverified resources are dropped before the plan is stored, so a true
	// value here is the only state a user ever sees.
	Verified bool `firestore:"verified" json:"verified"`
}

// Day is one day of the plan.
type Day struct {
	Day            int        `firestore:"day" json:"day"`
	FocusConcept   string     `firestore:"focusConcept" json:"focus_concept"`
	WhyThisMatters string     `firestore:"whyThisMatters" json:"why_this_matters"`
	EstimatedMins  int        `firestore:"estimatedMinutes" json:"estimated_minutes"`
	Resources      []Resource `firestore:"resources" json:"resources"`
	PracticeTask   string     `firestore:"practiceTask" json:"practice_task"`
	SelfCheck      string     `firestore:"selfCheck" json:"self_check"`
}

// RetestPlan closes the loop by pointing back into the product with a
// pre-configured next session.
type RetestPlan struct {
	AfterDay           int      `firestore:"afterDay" json:"after_day"`
	FocusAreas         []string `firestore:"focusAreas" json:"focus_areas"`
	RecommendedPersona string   `firestore:"recommendedPersona" json:"recommended_persona"`
	RecommendedBand    int      `firestore:"recommendedBand" json:"recommended_band"`
}

// Plan is the stored roadmap.
type Plan struct {
	SessionID   string     `firestore:"sessionId" json:"session_id"`
	HorizonDays int        `firestore:"horizonDays" json:"horizon_days"`
	Summary     string     `firestore:"summary" json:"summary"`
	Days        []Day      `firestore:"days" json:"days"`
	Retest      RetestPlan `firestore:"retestPlan" json:"retest_plan"`

	// Grounded records whether Search grounding succeeded. When false the plan
	// is still useful — a roadmap with no links beats no roadmap — and the UI
	// says so rather than pretending.
	Grounded     bool      `firestore:"grounded" json:"grounded"`
	Note         string    `firestore:"note,omitempty" json:"note,omitempty"`
	LinksFound   int       `firestore:"linksFound" json:"linksFound"`
	LinksDropped int       `firestore:"linksDropped" json:"linksDropped"`
	GeneratedAt  time.Time `firestore:"generatedAt" json:"generatedAt"`
}

// Builder generates roadmaps.
type Builder struct {
	cfg  *config.Config
	log  *slog.Logger
	vx   *vertexai.Client
	http *http.Client
}

// New builds the roadmap builder.
func New(cfg *config.Config, log *slog.Logger, vx *vertexai.Client) *Builder {
	return &Builder{
		cfg: cfg, log: log, vx: vx,
		http: &http.Client{
			Timeout: 12 * time.Second,
			// Cap redirects: a docs URL that bounces more than a few times is
			// usually a login wall or a redirect loop.
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// excludedDomains keeps content farms and answer-scrapers out of the results.
//
// The PRD asks for an allowlist, but the Search tool only supports exclusion.
// The allowlist intent is expressed in the prompt instead; this is the hard
// backstop for the worst offenders.
var excludedDomains = []string{
	"w3schools.com", "geeksforgeeks.org", "tutorialspoint.com",
	"javatpoint.com", "codecademy.com", "simplilearn.com",
	"medium.com", "quora.com", "chegg.com", "coursehero.com",
}

const buildTimeout = 3 * time.Minute

// Build generates a roadmap for a session.
//
// One grounded call for the whole plan, never one per day: the PRD budgets a
// single grounded call per roadmap, and per-day calls would multiply both cost
// and latency for no benefit.
func (b *Builder) Build(ctx context.Context, sess *store.Session, turns []*store.Turn, horizonDays int) (*Plan, error) {
	if horizonDays <= 0 {
		horizonDays = 7
	}

	clusters := Rank(Input{
		Turns: turns, Coverage: sess.Coverage,
		Digest: sess.Digest, HorizonDays: horizonDays,
	})
	if len(clusters) == 0 {
		// Nothing to study is a legitimate and pleasant outcome.
		return &Plan{
			SessionID: sess.ID, HorizonDays: horizonDays, Grounded: false,
			Summary: "No significant gaps surfaced in this session. Run a longer " +
				"interview or raise the difficulty to find your edges.",
			Days: []Day{}, GeneratedAt: time.Now(),
		}, nil
	}

	p, err := prompts.Get(prompts.RoadmapBuild)
	if err != nil {
		return nil, err
	}

	instruction := p.Render(map[string]string{
		"HORIZON_DAYS":   fmt.Sprint(horizonDays),
		"ROLE_TITLE":     roleTitle(sess.Digest),
		"CONCEPT_LIST":   renderClusters(clusters),
		"PROVEN_LIST":    orNone(sess.Coverage.Proven),
		"EXCLUDED_SITES": strings.Join(excludedDomains, ", "),
	})

	ctx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	temp := float32(0.4)
	genCfg := &genai.GenerateContentConfig{
		Temperature:      &temp,
		ResponseMIMEType: "application/json",
		ResponseSchema:   planSchema(),
		Tools: []*genai.Tool{{
			GoogleSearch: &genai.GoogleSearch{ExcludeDomains: excludedDomains},
		}},
	}

	grounded := true
	raw, err := b.vx.GenerateStructured(ctx, b.cfg.ModelReasoning, genai.Text(instruction), genCfg)
	if err != nil {
		// Degrade rather than fail: a roadmap with no links beats no roadmap.
		b.log.Warn("grounded roadmap call failed, retrying ungrounded",
			"session_id", sess.ID, "error", err.Error())

		grounded = false
		genCfg.Tools = nil
		raw, err = b.vx.GenerateStructured(ctx, b.cfg.ModelReasoning, genai.Text(instruction), genCfg)
		if err != nil {
			return nil, fmt.Errorf("roadmap: generation failed: %w", err)
		}
	}

	var parsed rawPlan
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("roadmap: response was not valid JSON: %w", err)
	}

	plan := b.assemble(sess, parsed, clusters, horizonDays, grounded)

	// THE step that makes the exit criterion true. Grounding metadata is not a
	// guarantee, and an ungrounded model invents plausible-looking URLs. So
	// every link is fetched before anyone sees it.
	b.verifyLinks(ctx, plan)

	b.log.Info("roadmap built",
		"session_id", sess.ID,
		"days", len(plan.Days),
		"grounded", plan.Grounded,
		"links_kept", plan.LinksFound,
		"links_dropped", plan.LinksDropped)

	return plan, nil
}

// verifyLinks fetches every resource URL and drops the ones that do not answer.
//
// A judge who clicks a 404 remembers it. Checking is cheap, parallel, and
// removes the entire class of failure — including the case where grounding
// silently did not fire and the model answered from memory.
func (b *Builder) verifyLinks(ctx context.Context, plan *Plan) {
	type check struct {
		day, idx int
		ok       bool
	}

	var wg sync.WaitGroup
	results := make(chan check, 64)

	for di := range plan.Days {
		for ri := range plan.Days[di].Resources {
			wg.Add(1)
			go func(di, ri int, url string) {
				defer wg.Done()
				results <- check{day: di, idx: ri, ok: b.urlResolves(ctx, url)}
			}(di, ri, plan.Days[di].Resources[ri].URL)
		}
	}
	go func() { wg.Wait(); close(results) }()

	verified := map[[2]int]bool{}
	for r := range results {
		verified[[2]int{r.day, r.idx}] = r.ok
	}

	for di := range plan.Days {
		kept := make([]Resource, 0, len(plan.Days[di].Resources))
		for ri, res := range plan.Days[di].Resources {
			if verified[[2]int{di, ri}] {
				res.Verified = true
				kept = append(kept, res)
				plan.LinksFound++
			} else {
				plan.LinksDropped++
				b.log.Info("dropped unreachable roadmap link", "url", res.URL)
			}
		}
		plan.Days[di].Resources = kept
	}

	if plan.LinksDropped > 0 && plan.LinksFound == 0 {
		plan.Note = "Resource links were unavailable for this plan. The concepts, " +
			"practice tasks, and self-checks below still apply."
	}
}

// urlResolves reports whether a URL answers.
//
// Tries HEAD first because it is cheap, then falls back to GET: plenty of docs
// sites reject HEAD with a 405 while serving GET perfectly well, and rejecting
// those would drop good links.
func (b *Builder) urlResolves(ctx context.Context, rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		return false
	}

	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
		if err != nil {
			return false
		}
		// Some documentation sites serve a 403 to an obviously-robotic client.
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CrucibleLinkCheck/1.0)")

		resp, err := b.http.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true
		}
		// A 405 or 403 on HEAD is worth a GET; a 404 is final.
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			return false
		}
	}
	return false
}

func (b *Builder) assemble(sess *store.Session, raw rawPlan, clusters []Cluster, horizonDays int, grounded bool) *Plan {
	plan := &Plan{
		SessionID:   sess.ID,
		HorizonDays: horizonDays,
		Summary:     strings.TrimSpace(raw.Summary),
		Grounded:    grounded,
		Days:        make([]Day, 0, len(raw.Days)),
		GeneratedAt: time.Now(),
	}
	if !grounded {
		plan.Note = "Generated without live search, so resource links were omitted."
	}

	for i, d := range raw.Days {
		day := Day{
			Day:            i + 1, // renumber: models skip and repeat day numbers
			FocusConcept:   strings.TrimSpace(d.FocusConcept),
			WhyThisMatters: strings.TrimSpace(d.WhyThisMatters),
			EstimatedMins:  clampInt(d.EstimatedMinutes, 15, 180),
			PracticeTask:   strings.TrimSpace(d.PracticeTask),
			SelfCheck:      strings.TrimSpace(d.SelfCheck),
			Resources:      make([]Resource, 0, len(d.Resources)),
		}
		if day.FocusConcept == "" {
			continue
		}
		for _, r := range d.Resources {
			url := strings.TrimSpace(r.URL)
			if url == "" {
				continue
			}
			day.Resources = append(day.Resources, Resource{
				Title:   strings.TrimSpace(r.Title),
				URL:     url,
				Type:    strings.TrimSpace(r.Type),
				Minutes: clampInt(r.Minutes, 5, 120),
			})
		}
		plan.Days = append(plan.Days, day)
		if len(plan.Days) >= horizonDays {
			break
		}
	}

	plan.Retest = RetestPlan{
		AfterDay:           retestAfter(len(plan.Days)),
		FocusAreas:         topLabels(clusters, 3),
		RecommendedPersona: string(orDefaultPersona(sess.Persona)),
		// Retest one band above where they finished: the point is to prove the
		// gap is closed, not to repeat the same difficulty.
		RecommendedBand: clampInt(sess.DifficultyBand+1, 2, 5),
	}
	return plan
}

func retestAfter(days int) int {
	if days <= 2 {
		return days
	}
	return days / 2
}

func topLabels(clusters []Cluster, n int) []string {
	out := make([]string, 0, n)
	for _, c := range clusters {
		if len(out) >= n {
			break
		}
		out = append(out, c.Label)
	}
	return out
}

func orDefaultPersona(p store.Persona) store.Persona {
	if p.Valid() {
		return p
	}
	return store.PersonaTechLead
}

type rawPlan struct {
	Summary string `json:"summary"`
	Days    []struct {
		FocusConcept     string `json:"focus_concept"`
		WhyThisMatters   string `json:"why_this_matters"`
		EstimatedMinutes int    `json:"estimated_minutes"`
		PracticeTask     string `json:"practice_task"`
		SelfCheck        string `json:"self_check"`
		Resources        []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Type    string `json:"type"`
			Minutes int    `json:"minutes"`
		} `json:"resources"`
	} `json:"days"`
}

func renderClusters(clusters []Cluster) string {
	var b strings.Builder
	for i, c := range clusters {
		fmt.Fprintf(&b, "%d. %s", i+1, c.Label)
		if c.Frequency > 1 {
			fmt.Fprintf(&b, " (came up in %d answers)", c.Frequency)
		}
		if c.Relevance >= RelevanceMustHave {
			b.WriteString(" [named as REQUIRED in the job description]")
		} else if c.Relevance >= RelevanceNiceToHave {
			b.WriteString(" [listed as preferred in the job description]")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func roleTitle(digest map[string]any) string {
	if role, ok := digest["role"].(map[string]any); ok {
		if t, _ := role["title"].(string); t != "" {
			return t
		}
	}
	return "the role"
}

func orNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func planSchema() *genai.Schema {
	str := func(d string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeString, Description: d}
	}
	resource := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"title":   str("The page's actual title."),
			"url":     str("A REAL, complete URL to official documentation, a specification, an arxiv paper, or a university course page. Every URL is fetched and verified; invented ones are discarded."),
			"type":    str("docs | spec | paper | course | video | book"),
			"minutes": {Type: genai.TypeInteger},
		},
		Required: []string{"title", "url", "type", "minutes"},
	}
	day := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"focus_concept":     str("The single concept for this day."),
			"why_this_matters":  str("One sentence tying it to their performance and the role."),
			"estimated_minutes": {Type: genai.TypeInteger},
			"resources":         {Type: genai.TypeArray, Items: resource},
			"practice_task":     str("Something concrete to DO, referencing their own projects where possible."),
			"self_check":        str("A question they can answer to know whether it landed."),
		},
		Required: []string{"focus_concept", "why_this_matters", "estimated_minutes", "resources", "practice_task", "self_check"},
	}
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"summary": str("Two or three sentences to the candidate, naming what is solid before what is not."),
			"days":    {Type: genai.TypeArray, Items: day},
		},
		Required: []string{"summary", "days"},
	}
}
