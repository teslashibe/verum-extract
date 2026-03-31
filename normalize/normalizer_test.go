package normalize

import (
	"testing"

	"github.com/teslashibe/verum-extract/compounds"
)

func TestCanonicalize(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"oral BPC", "BPC-157"},
		{"BPC/TB", "BPC-157"},
		{"BPC157 oral", "BPC-157"},
		{"BPC nasal spray", "BPC-157"},
		{"liquid BPC", "BPC-157"},
		{"NA selank", "Selank"},
		{"N-acetyl-selank", "Selank"},
		{"NA Semax Amidate", "Semax"},
		{"n-acetyl amidate Semax", "Semax"},
		{"grey tirzepatide", "tirzepatide"},
		{"GHK-cu topical", "ghk-cu"},
		{"GHK-cu injections", "ghk-cu"},
		{"CJC no dac", "CJC-1295"},
		{"hgh secretagogues", "HGH"},
		{"HGH Frag", "HGH Fragment 176-191"},
		{"compounded semaglutide", "semaglutide"},
		{"Ozempic semaglutide", "semaglutide"},
		{"Thymosin Beta-4", "TB-500"},
		{"tb4", "TB-500"},
	}
	for _, tc := range cases {
		got := canonicalize(tc.input)
		if got != tc.want {
			t.Errorf("canonicalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizerMergesDuplicates(t *testing.T) {
	registry := compounds.Default()

	n := New(registry, WithAutoRegister())

	variants := []string{
		"Selank",
		"NA selank",
		"N-acetyl-selank",
		"selank injection",
		"selank nasal spray",
		"Selank injecting",
	}

	for _, v := range variants {
		c := n.match(v)
		if c == nil {
			t.Errorf("match(%q) = nil, want Selank", v)
			continue
		}
		if c.Name != "selank" {
			t.Errorf("match(%q).Name = %q, want %q", v, c.Name, "selank")
		}
	}
}
