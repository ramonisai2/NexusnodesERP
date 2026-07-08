package engine

import (
	"math"
	"sort"
	"sync"
	"time"
)

type Kind string

const (
	KindProduct Kind = "product"
	KindLabel   Kind = "label"
	KindPhoto   Kind = "photo"
)

// Document is a searchable unit projected from OLTP sources.
type Document struct {
	ID        string            `json:"id"`
	Kind      Kind              `json:"kind"`
	OrgID     string            `json:"org_id"`
	BranchID  string            `json:"branch_id,omitempty"`
	Title     string            `json:"title"`
	Subtitle  string            `json:"subtitle,omitempty"`
	Body      string            `json:"body,omitempty"`
	SKU       string            `json:"sku,omitempty"`
	Barcode   string            `json:"barcode,omitempty"`
	Href      string            `json:"href,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

type Hit struct {
	Document
	Score   float64  `json:"score"`
	Snippet string   `json:"snippet,omitempty"`
	Matched []string `json:"matched,omitempty"`
}

type Stats struct {
	Documents int            `json:"documents"`
	Terms     int            `json:"terms"`
	ByKind    map[string]int `json:"by_kind"`
	IndexedAt time.Time      `json:"indexed_at"`
}

type posting struct {
	doc   int
	tf    float64
	field float64 // field boost accumulator
}

type Index struct {
	mu        sync.RWMutex
	docs      []Document
	tokens    [][]string
	lengths   []float64
	inv       map[string][]posting
	avgLen    float64
	indexedAt time.Time
}

func NewIndex() *Index {
	return &Index{inv: map[string][]posting{}}
}

// Rebuild replaces the whole inverted index (search-engine style projection).
func (idx *Index) Rebuild(docs []Document) {
	tokens := make([][]string, len(docs))
	lengths := make([]float64, len(docs))
	inv := make(map[string][]posting, 1024)
	var sum float64

	for i, d := range docs {
		weighted := weightedText(d)
		toks := DetectiveMode(weighted.text)
		tokens[i] = toks
		lengths[i] = float64(len(toks))
		sum += lengths[i]

		tf := map[string]float64{}
		boost := map[string]float64{}
		for _, t := range DetectiveMode(d.Title + " " + d.SKU + " " + d.Barcode) {
			tf[t]++
			boost[t] = math.Max(boost[t], 3.0)
		}
		for _, t := range DetectiveMode(d.Subtitle) {
			tf[t]++
			boost[t] = math.Max(boost[t], 2.0)
		}
		for _, t := range DetectiveMode(d.Body) {
			tf[t]++
			boost[t] = math.Max(boost[t], 1.0)
		}
		for t, c := range tf {
			inv[t] = append(inv[t], posting{doc: i, tf: c, field: boost[t]})
		}
	}

	avg := 1.0
	if len(docs) > 0 {
		avg = sum / float64(len(docs))
		if avg < 1 {
			avg = 1
		}
	}

	idx.mu.Lock()
	idx.docs = docs
	idx.tokens = tokens
	idx.lengths = lengths
	idx.inv = inv
	idx.avgLen = avg
	idx.indexedAt = time.Now().UTC()
	idx.mu.Unlock()
}

type weightedBlob struct{ text string }

func weightedText(d Document) weightedBlob {
	// Title/SKU repeated so tokenizer + BM25 see stronger signal without separate fields.
	parts := []string{
		d.Title, d.Title, d.Title,
		d.SKU, d.SKU,
		d.Barcode, d.Barcode,
		d.Subtitle, d.Subtitle,
		d.Body,
	}
	var b stringsBuilder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(p)
		b.WriteByte(' ')
	}
	return weightedBlob{text: b.String()}
}

// tiny local builder to avoid importing strings in hot path file twice awkwardly
type stringsBuilder struct {
	b []byte
}

func (s *stringsBuilder) WriteString(v string) { s.b = append(s.b, v...) }
func (s *stringsBuilder) WriteByte(c byte)     { s.b = append(s.b, c) }
func (s *stringsBuilder) String() string       { return string(s.b) }

type Query struct {
	Text     string
	OrgID    string
	BranchID string
	Kinds    []Kind
	Limit    int
}

func (idx *Index) Search(q Query) []Hit {
	terms := DetectiveMode(q.Text)
	if len(terms) == 0 {
		return nil
	}
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 50 {
		q.Limit = 50
	}
	kindOK := map[Kind]bool{}
	for _, k := range q.Kinds {
		kindOK[k] = true
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	N := float64(len(idx.docs))
	if N == 0 {
		return nil
	}

	const k1 = 1.2
	const b = 0.75

	scores := map[int]float64{}
	matched := map[int]map[string]struct{}{}

	for _, term := range terms {
		postings := idx.inv[term]
		df := float64(len(postings))
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (N-df+0.5)/(df+0.5))
		for _, p := range postings {
			doc := idx.docs[p.doc]
			if !orgMatch(q.OrgID, doc) {
				continue
			}
			if q.BranchID != "" && doc.BranchID != "" && doc.BranchID != q.BranchID {
				continue
			}
			if len(kindOK) > 0 && !kindOK[doc.Kind] {
				continue
			}
			dl := idx.lengths[p.doc]
			tfNorm := (p.tf * (k1 + 1)) / (p.tf + k1*(1-b+b*(dl/idx.avgLen)))
			scores[p.doc] += idf * tfNorm * math.Max(p.field, 1)
			if matched[p.doc] == nil {
				matched[p.doc] = map[string]struct{}{}
			}
			matched[p.doc][term] = struct{}{}
		}
	}

	// Exact / prefix boosts for barcode & SKU (search-engine style hard matches).
	raw := BatComputer(q.Text)
	for i, doc := range idx.docs {
		if !orgMatch(q.OrgID, doc) {
			continue
		}
		if q.BranchID != "" && doc.BranchID != "" && doc.BranchID != q.BranchID {
			continue
		}
		if len(kindOK) > 0 && !kindOK[doc.Kind] {
			continue
		}
		if doc.Barcode != "" && BatComputer(doc.Barcode) == raw {
			scores[i] += 25
		} else if doc.SKU != "" && BatComputer(doc.SKU) == raw {
			scores[i] += 18
		} else if hasPrefixToken(doc.Title, q.Text) {
			scores[i] += 4
		}
	}

	type pair struct {
		i int
		s float64
	}
	ranked := make([]pair, 0, len(scores))
	for i, s := range scores {
		if s <= 0 {
			continue
		}
		ranked = append(ranked, pair{i: i, s: s})
	}
	sort.Slice(ranked, func(a, b int) bool {
		if ranked[a].s == ranked[b].s {
			return idx.docs[ranked[a].i].Title < idx.docs[ranked[b].i].Title
		}
		return ranked[a].s > ranked[b].s
	})
	if len(ranked) > q.Limit {
		ranked = ranked[:q.Limit]
	}

	hits := make([]Hit, 0, len(ranked))
	for _, r := range ranked {
		doc := idx.docs[r.i]
		ms := make([]string, 0, len(matched[r.i]))
		for t := range matched[r.i] {
			ms = append(ms, t)
		}
		sort.Strings(ms)
		hits = append(hits, Hit{
			Document: doc,
			Score:    math.Round(r.s*1000) / 1000,
			Snippet:  snippet(doc),
			Matched:  ms,
		})
	}
	return hits
}

func (idx *Index) Stats() Stats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	by := map[string]int{}
	for _, d := range idx.docs {
		by[string(d.Kind)]++
	}
	return Stats{
		Documents: len(idx.docs),
		Terms:     len(idx.inv),
		ByKind:    by,
		IndexedAt: idx.indexedAt,
	}
}

func snippet(d Document) string {
	if d.Subtitle != "" {
		return d.Subtitle
	}
	if d.Body != "" {
		if len(d.Body) > 140 {
			return d.Body[:140] + "…"
		}
		return d.Body
	}
	return d.Title
}

func orgMatch(claimOrg string, doc Document) bool {
	if claimOrg == "" {
		return true
	}
	if doc.OrgID == claimOrg {
		return true
	}
	if code := doc.Attrs["org_code"]; code != "" && code == claimOrg {
		return true
	}
	return false
}
