package encounter

import (
	"encoding/json"
	"html/template"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"chikitsalaya/internal/opd/masters"
)

// lookupOptionJSON is what a dosage/duration picklist entry marshals to
// for the client-side dropdown fields.
type lookupOptionJSON struct {
	ID    int64  `json:"id"`
	Value string `json:"value"`
}

// toLookupOptionJSON maps a picklist to its JSON-marshalable shape, shared
// by lookupValuesJSON (page-embed) and QuickAddLookupValueHandler (a raw
// JSON response after an inline add).
func toLookupOptionJSON(values []masters.LookupValue) []lookupOptionJSON {
	out := make([]lookupOptionJSON, len(values))
	for i, v := range values {
		out[i] = lookupOptionJSON{ID: v.ID, Value: v.Value}
	}
	return out
}

// lookupValuesJSON marshals a picklist to a JSON array embedded directly in
// the page. template.JS marks it pre-escaped so html/template doesn't
// re-escape valid JSON inside a <script> block.
func lookupValuesJSON(values []masters.LookupValue) template.JS {
	b, err := json.Marshal(toLookupOptionJSON(values))
	if err != nil {
		return template.JS("[]")
	}
	return template.JS(b)
}

// durationUnitRank orders duration values by unit magnitude (shortest
// first) instead of lookup_values.sort_order, which interleaves hours and
// days.
var durationUnitRank = map[string]int{
	"hour": 1, "day": 2, "week": 3, "month": 4,
}

var durationPattern = regexp.MustCompile(`(?i)^(\d+)\s*(hour|day|week|month)s?$`)

// durationRank returns a sort key and whether value matched the
// "<number> <unit>" shape at all; non-matches sort last.
func durationRank(value string) (int, bool) {
	m := durationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	unitRank, ok := durationUnitRank[strings.ToLower(m[2])]
	if !ok {
		return 0, false
	}
	return unitRank*100000 + n, true
}

// sortDurationOptions reorders values hours -> days -> weeks -> months for
// display only; the underlying lookup_values.sort_order is left untouched.
func sortDurationOptions(values []masters.LookupValue) []masters.LookupValue {
	out := make([]masters.LookupValue, len(values))
	copy(out, values)
	sort.SliceStable(out, func(i, j int) bool {
		ri, oki := durationRank(out[i].Value)
		rj, okj := durationRank(out[j].Value)
		if oki != okj {
			return oki
		}
		if !oki {
			return false
		}
		return ri < rj
	})
	return out
}
