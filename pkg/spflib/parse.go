package spflib

import (
	"fmt"
	"strings"

	"bytes"

	"io"

	"github.com/pkg/errors"
)

// SPFRecord stores the parts of an SPF record.
type SPFRecord struct {
	Parts []*SPFPart
}

// Lookups returns the number of DNS lookups required by s.
func (s *SPFRecord) Lookups() int {
	count := 0
	for _, p := range s.Parts {
		if p.IsLookup {
			count++
		}
		if p.IncludeRecord != nil {
			count += p.IncludeRecord.Lookups()
		}
	}
	return count
}

// SPFPart stores a part of an SPF record, with attributes.
type SPFPart struct {
	Text          string
	IsLookup      bool
	IncludeRecord *SPFRecord
	IncludeDomain string
}

var qualifiers = map[byte]bool{
	'?': true,
	'~': true,
	'-': true,
	'+': true,
}

// Parse parses a raw SPF record.
func Parse(text string, dnsres Resolver) (*SPFRecord, error) {
	if !strings.HasPrefix(text, "v=spf1 ") {
		return nil, errors.Errorf("Not an spf record")
	}
	parts := strings.Split(text, " ")
	rec := &SPFRecord{}
	for pi, part := range parts[1:] {
		if part == "" {
			continue
		}
		p := &SPFPart{Text: part}
		if qualifiers[part[0]] {
			part = part[1:]
		}
		rec.Parts = append(rec.Parts, p)
		if part == "all" {
			// all. nothing else matters.
			break
		} else if strings.HasPrefix(part, "a") || strings.HasPrefix(part, "mx") {
			p.IsLookup = true
		} else if strings.HasPrefix(part, "ip4:") || strings.HasPrefix(part, "ip6:") {
			// ip address, 0 lookups
			continue
		} else if strings.HasPrefix(part, "include:") || strings.HasPrefix(part, "redirect:") {
			if strings.HasPrefix(part, "redirect:") {
				// pi + 2: because pi starts at 0 when it iterates starting on parts[1],
				// and because len(parts) is one bigger than the highest index.
				if (pi + 2) != len(parts) {
					return nil, errors.Errorf("%s must be last item", part)
				}
				p.IncludeDomain = strings.TrimPrefix(part, "redirect:")
			} else {
				p.IncludeDomain = strings.TrimPrefix(part, "include:")
			}
			p.IsLookup = true
			if dnsres != nil {
				subRecord, err := dnsres.GetSPF(p.IncludeDomain)
				if err != nil {
					return nil, err
				}
				p.IncludeRecord, err = Parse(subRecord, dnsres)
				if err != nil {
					return nil, errors.Errorf("In included spf: %s", err)
				}
			}
		} else if strings.HasPrefix(part, "exists:") || strings.HasPrefix(part, "ptr:") {
			p.IsLookup = true
		} else {
			return nil, errors.Errorf("Unsupported spf part %s", part)
		}

	}
	return rec, nil
}

func dump(rec *SPFRecord, indent string, w io.Writer) {

	fmt.Fprintf(w, "%sTotal Lookups: %d\n", indent, rec.Lookups())
	fmt.Fprint(w, indent+"v=spf1")
	for _, p := range rec.Parts {
		fmt.Fprint(w, " "+p.Text)
	}
	fmt.Fprintln(w)
	indent += "\t"
	for _, p := range rec.Parts {
		if p.IsLookup {
			fmt.Fprintln(w, indent+p.Text)
		}
		if p.IncludeRecord != nil {
			dump(p.IncludeRecord, indent+"\t", w)
		}
	}
}

// Print prints an SPFRecord.
func (s *SPFRecord) Print() string {
	w := &bytes.Buffer{}
	dump(s, "", w)
	return w.String()
}
