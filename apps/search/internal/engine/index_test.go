package engine

import "testing"

func TestTokenizeAccentFold(t *testing.T) {
	toks := Tokenize("Agua de Piña Ñandú")
	joined := ""
	for _, x := range toks {
		joined += " " + x
	}
	for _, want := range []string{"agua", "pina", "nandu"} {
		found := false
		for _, tok := range toks {
			if tok == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %v", want, toks)
		}
	}
}

func TestBM25RanksTitleHigher(t *testing.T) {
	idx := NewIndex()
	idx.Rebuild([]Document{
		{ID: "1", Kind: KindProduct, OrgID: "o", Title: "Aceite vegetal 1L", Body: "cocina"},
		{ID: "2", Kind: KindProduct, OrgID: "o", Title: "Jabón líquido", Body: "aceite esencial aroma"},
		{ID: "3", Kind: KindLabel, OrgID: "o", BranchID: "br", Title: "Aceite", SKU: "ACE-1", Barcode: "750123"},
	})
	hits := idx.Search(Query{Text: "aceite", OrgID: "o", Limit: 10})
	if len(hits) < 2 {
		t.Fatalf("expected hits, got %d", len(hits))
	}
	if hits[0].ID != "3" && hits[0].ID != "1" {
		t.Fatalf("unexpected top hit %#v", hits[0])
	}
	// barcode exact
	hits = idx.Search(Query{Text: "750123", OrgID: "o", Limit: 5})
	if len(hits) == 0 || hits[0].ID != "3" {
		t.Fatalf("barcode should win: %#v", hits)
	}
}
